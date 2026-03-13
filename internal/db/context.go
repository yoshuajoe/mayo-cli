package db

import (
	"os"
)

func ReadContextFile() (string, error) {
	path := "CONTEXT.md"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func GetWorkingDirectory() string {
	dir, _ := os.Getwd()
	return dir
}
