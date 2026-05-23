//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	disableMaxPrivilege = 0x1
	lowIntegritySID     = "S-1-16-4096"
	waitStillActive     = 259
)

var procCreateRestrictedToken = windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken")

type windowsRunner struct{}

func NewRunner() Runner {
	return windowsRunner{}
}

func (windowsRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if strings.TrimSpace(spec.Executable) == "" {
		return Result{}, fmt.Errorf("sandbox executable is required")
	}

	workspaceDir, err := cleanWindowsAbsPath(spec.WorkspaceDir)
	if err != nil {
		return Result{}, fmt.Errorf("sandbox workspace_dir: %w", err)
	}
	if err := prepareWindowsWorkspace(ctx, workspaceDir); err != nil {
		return Result{}, err
	}

	token, err := newLowIntegrityRestrictedToken()
	if err != nil {
		return Result{}, err
	}
	defer token.Close()

	job, err := newKillOnCloseJob()
	if err != nil {
		return Result{}, err
	}
	defer windows.CloseHandle(job)

	stdoutPipe, err := newInheritablePipe()
	if err != nil {
		return Result{}, fmt.Errorf("create stdout pipe: %w", err)
	}
	defer stdoutPipe.closeRead()
	defer stdoutPipe.closeWrite()

	stderrPipe, err := newInheritablePipe()
	if err != nil {
		return Result{}, fmt.Errorf("create stderr pipe: %w", err)
	}
	defer stderrPipe.closeRead()
	defer stderrPipe.closeWrite()

	stdin, err := openWindowsNullDevice()
	if err != nil {
		return Result{}, fmt.Errorf("open stdin null device: %w", err)
	}
	defer windows.CloseHandle(stdin)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var copyWG sync.WaitGroup
	copyWG.Add(2)
	go readPipe(&copyWG, stdoutPipe.read, &stdout)
	go readPipe(&copyWG, stderrPipe.read, &stderr)

	pi, err := startRestrictedProcess(token, workspaceDir, spec, stdin, stdoutPipe.write, stderrPipe.write)
	stdoutPipe.closeWrite()
	stderrPipe.closeWrite()
	if err != nil {
		copyWG.Wait()
		return Result{}, err
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)

	assigned := false
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		copyWG.Wait()
		return Result{}, fmt.Errorf("assign process to job object: %w", err)
	}
	assigned = true

	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		terminateWindowsSandbox(job, pi.Process, assigned)
		copyWG.Wait()
		return Result{}, fmt.Errorf("resume sandbox process: %w", err)
	}

	exitCode, waitErr := waitWindowsProcess(ctx, job, pi.Process, assigned)
	copyWG.Wait()

	result := Result{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	if waitErr != nil {
		return result, waitErr
	}
	return result, nil
}

func prepareWindowsWorkspace(ctx context.Context, workspaceDir string) error {
	cmd := exec.CommandContext(ctx, "icacls", workspaceDir, "/setintegritylevel", "(OI)(CI)L")
	if output, err := cmd.CombinedOutput(); err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return fmt.Errorf("prepare windows sandbox workspace: %w", err)
		}
		return fmt.Errorf("prepare windows sandbox workspace: %w: %s", err, text)
	}
	return nil
}

func newLowIntegrityRestrictedToken() (windows.Token, error) {
	var current windows.Token
	access := uint32(windows.TOKEN_DUPLICATE | windows.TOKEN_ASSIGN_PRIMARY | windows.TOKEN_QUERY | windows.TOKEN_ADJUST_DEFAULT | windows.TOKEN_ADJUST_SESSIONID)
	if err := windows.OpenProcessToken(windows.CurrentProcess(), access, &current); err != nil {
		return 0, fmt.Errorf("open process token: %w", err)
	}
	defer current.Close()

	restricted, err := createRestrictedToken(current)
	if err != nil {
		return 0, err
	}

	lowSID, err := windows.StringToSid(lowIntegritySID)
	if err != nil {
		restricted.Close()
		return 0, fmt.Errorf("create low integrity sid: %w", err)
	}

	label := windows.Tokenmandatorylabel{
		Label: windows.SIDAndAttributes{
			Sid:        lowSID,
			Attributes: windows.SE_GROUP_INTEGRITY,
		},
	}
	if err := windows.SetTokenInformation(
		restricted,
		windows.TokenIntegrityLevel,
		(*byte)(unsafe.Pointer(&label)),
		label.Size(),
	); err != nil {
		restricted.Close()
		return 0, fmt.Errorf("set token low integrity level: %w", err)
	}

	return restricted, nil
}

func createRestrictedToken(token windows.Token) (windows.Token, error) {
	var restricted windows.Token
	r1, _, err := procCreateRestrictedToken.Call(
		uintptr(token),
		uintptr(disableMaxPrivilege),
		0,
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&restricted)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return 0, fmt.Errorf("create restricted token: %w", err)
		}
		return 0, errors.New("create restricted token failed")
	}
	return restricted, nil
}

func newKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create job object: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("configure job object: %w", err)
	}
	return job, nil
}

type windowsPipe struct {
	read  windows.Handle
	write windows.Handle
}

func newInheritablePipe() (windowsPipe, error) {
	sa := windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}
	var read windows.Handle
	var write windows.Handle
	if err := windows.CreatePipe(&read, &write, &sa, 0); err != nil {
		return windowsPipe{}, err
	}
	if err := windows.SetHandleInformation(read, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		windows.CloseHandle(read)
		windows.CloseHandle(write)
		return windowsPipe{}, err
	}
	return windowsPipe{read: read, write: write}, nil
}

func (p *windowsPipe) closeRead() {
	if p.read != 0 {
		windows.CloseHandle(p.read)
		p.read = 0
	}
}

func (p *windowsPipe) closeWrite() {
	if p.write != 0 {
		windows.CloseHandle(p.write)
		p.write = 0
	}
}

func openWindowsNullDevice() (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString("NUL")
	if err != nil {
		return 0, err
	}
	sa := windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}
	return windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		&sa,
		windows.OPEN_EXISTING,
		0,
		0,
	)
}

func readPipe(wg *sync.WaitGroup, handle windows.Handle, dst *bytes.Buffer) {
	defer wg.Done()

	buf := make([]byte, 4096)
	for {
		var n uint32
		err := windows.ReadFile(handle, buf, &n, nil)
		if n > 0 {
			dst.Write(buf[:n])
		}
		if err != nil {
			if errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_HANDLE_EOF) || errors.Is(err, windows.ERROR_NO_DATA) {
				return
			}
			return
		}
		if n == 0 {
			return
		}
	}
}

func startRestrictedProcess(token windows.Token, workspaceDir string, spec Spec, stdin windows.Handle, stdout windows.Handle, stderr windows.Handle) (windows.ProcessInformation, error) {
	args := append([]string{strings.TrimSpace(spec.Executable)}, spec.Args...)
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("build command line: %w", err)
	}
	currentDir, err := windows.UTF16PtrFromString(workspaceDir)
	if err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("build current directory: %w", err)
	}
	envBlock, err := windowsEnvironmentBlock(token, spec.Env)
	if err != nil {
		return windows.ProcessInformation{}, err
	}
	defer envBlock.close()

	attributeList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("create process attribute list: %w", err)
	}
	defer attributeList.Delete()

	inheritedHandles := []windows.Handle{stdin, stdout, stderr}
	if err := attributeList.Update(
		windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
		unsafe.Pointer(&inheritedHandles[0]),
		uintptr(len(inheritedHandles))*unsafe.Sizeof(inheritedHandles[0]),
	); err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("configure inherited handles: %w", err)
	}

	startup := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:        uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags:     windows.STARTF_USESTDHANDLES,
			StdInput:  stdin,
			StdOutput: stdout,
			StdErr:    stderr,
		},
		ProcThreadAttributeList: attributeList.List(),
	}
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW | windows.EXTENDED_STARTUPINFO_PRESENT)
	if err := windows.CreateProcessAsUser(
		token,
		nil,
		commandLine,
		nil,
		nil,
		true,
		flags,
		envBlock.ptr(),
		currentDir,
		&startup.StartupInfo,
		&pi,
	); err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("create restricted process: %w", err)
	}
	return pi, nil
}

type environmentBlock struct {
	ptrValue *uint16
	data     []uint16
	closeFn  func()
}

func (b environmentBlock) ptr() *uint16 {
	if b.ptrValue != nil {
		return b.ptrValue
	}
	if len(b.data) == 0 {
		return nil
	}
	return &b.data[0]
}

func (b environmentBlock) close() {
	if b.closeFn != nil {
		b.closeFn()
	}
}

func windowsEnvironmentBlock(token windows.Token, extra map[string]string) (environmentBlock, error) {
	if len(extra) == 0 {
		var block *uint16
		if err := windows.CreateEnvironmentBlock(&block, token, true); err != nil {
			return environmentBlock{}, fmt.Errorf("create environment block: %w", err)
		}
		return environmentBlock{
			ptrValue: block,
			closeFn:  func() { windows.DestroyEnvironmentBlock(block) },
		}, nil
	}

	env := append(os.Environ(), windowsEnvList(extra)...)
	block, err := createWindowsEnvironmentBlock(env)
	if err != nil {
		return environmentBlock{}, fmt.Errorf("create environment block: %w", err)
	}
	return environmentBlock{data: block}, nil
}

func createWindowsEnvironmentBlock(env []string) ([]uint16, error) {
	if len(env) == 0 {
		return []uint16{0, 0}, nil
	}

	block := make([]uint16, 0, len(env)+1)
	for _, item := range env {
		encoded, err := windows.UTF16FromString(item)
		if err != nil {
			return nil, err
		}
		block = append(block, encoded...)
	}
	block = append(block, 0)
	return block, nil
}

func windowsEnvList(env map[string]string) []string {
	items := make([]string, 0, len(env))
	for key, value := range env {
		items = append(items, key+"="+value)
	}
	return items
}

func waitWindowsProcess(ctx context.Context, job windows.Handle, process windows.Handle, assigned bool) (int, error) {
	done := make(chan error, 1)
	go func() {
		_, err := windows.WaitForSingleObject(process, windows.INFINITE)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return -1, fmt.Errorf("wait sandbox process: %w", err)
		}
	case <-ctx.Done():
		terminateWindowsSandbox(job, process, assigned)
		<-done
		return -1, ctx.Err()
	}

	var code uint32
	if err := windows.GetExitCodeProcess(process, &code); err != nil {
		return -1, fmt.Errorf("get sandbox exit code: %w", err)
	}
	if code == waitStillActive {
		return -1, errors.New("sandbox process is still active after wait")
	}
	return int(code), nil
}

func terminateWindowsSandbox(job windows.Handle, process windows.Handle, assigned bool) {
	if assigned {
		_ = windows.TerminateJobObject(job, 1)
		return
	}
	_ = windows.TerminateProcess(process, 1)
}

func cleanWindowsAbsPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	full, err := windows.FullPath(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(full), nil
}
