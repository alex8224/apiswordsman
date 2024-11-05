package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openDebugWin() {
	url := fmt.Sprintf("http://localhost:%d/static/index.html", default_port)
	switch runtime.GOOS {
	case "windows":
		err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		if err != nil {
			fmt.Printf("Error opening browser on Windows: %v\n", err)
		}
	case "darwin":
		err := exec.Command("open", url).Start()
		if err != nil {
			fmt.Printf("Error opening browser on macOS: %v\n", err)
		}
	case "linux":
		err := exec.Command("xdg-open", url).Start()
		if err != nil {
			fmt.Printf("Error opening browser on Linux: %v\n", err)
		}
	default:
		fmt.Println("Unsupported platform", runtime.GOOS)
	}
}
