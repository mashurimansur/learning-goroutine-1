#!/usr/bin/env bash
rm -f output.csv plot-golang.png
mkdir -p dist
go mod tidy
go build -o dist/goroutine learning-goroutine
psrecord './dist/goroutine' --include-children --plot plot-golang.png