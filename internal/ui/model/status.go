package model

import "strings"

type statusModel struct {
	message string
}

func newStatusModel() statusModel {
	return statusModel{}
}

func (m statusModel) setMessage(message string) statusModel {
	m.message = strings.TrimSpace(message)
	return m
}

func (m statusModel) View() string {
	return m.message
}
