package analyzer

import (
	`os`
	`path/filepath`
	`testing`
)

func TestDetectStack(t *testing.T) {
	tests := []struct {
		lines    []string
		expected string
	}{
		{[]string{"FROM golang:1.21", "RUN go build"}, "go"},
		{[]string{"FROM openjdk:17", "RUN java -jar app.jar"}, "java"},
		{[]string{"FROM python:3.10", "RUN pip install flask"}, "python"},
		{[]string{"FROM node:18", "RUN npm install"}, "node"},
		{[]string{"FROM rust:1.82", "RUN cargo build"}, "rust"},
		{[]string{"FROM mcr.microsoft.com/dotnet/aspnet:7.0"}, "dotnet"},
		{[]string{"FROM php:8.2", "RUN composer install"}, "php"},
		{[]string{"FROM ruby:3.2", "RUN bundle install"}, "ruby"},
		{[]string{"FROM ubuntu:latest"}, "generic"},
	}

	for _, tt := range tests {
		got := DetectStack(tt.lines)
		if got != tt.expected {
			t.Errorf("DetectStack(%v) = %q; want %q", tt.lines, got, tt.expected)
		}
	}
}

func TestDetectStackFromFile(t *testing.T) {
	// Create a temporary Dockerfile
	content := `FROM python:3.10
RUN pip install flask`
	tmpFile := filepath.Join(os.TempDir(), "Dockfile_test_detectstack")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp Dockerfile: %v", err)
	}
	defer os.Remove(tmpFile) // clean up

	// Valid case
	stack := DetectStackFromFile(tmpFile)
	if stack != "python" {
		t.Errorf("Expected stack 'python', got '%s'", stack)
	}

	// Invalid case: file does not exist
	invalid := DetectStackFromFile("nonexistent/path/Dockerfile")
	if invalid != "generic" {
		t.Errorf("Expected 'generic' for missing file, got '%s'", invalid)
	}
}
