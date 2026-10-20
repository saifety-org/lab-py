//go:build !onnx

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "context-onnx-bridge requires -tags onnx"); os.Exit(1) }
