package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
	"github.com/getlantern/systray"
	"github.com/getlantern/systray/example/icon"
)

const flashInterval = 200 * time.Millisecond

type tray struct {
}

func (tray *tray) RunTray(onReady func(), onExit func()) {
	systray.Run(func() { tray.InitTray(); go onReady() }, onExit)
}

func MakeTray() *tray {
	return &tray{}
}

const trayToopTip string = "APIXIA客户端"

func (tray *tray) InitTray() {
	systray.SetIcon(icon.Data)
	systray.SetTitle("APIXIA")
	systray.SetTooltip(trayToopTip)

	mDebug := systray.AddMenuItem("打开调试页", "打开调试页")
	// 你可以添加一个分隔符
	systray.AddSeparator()

	mQuit := systray.AddMenuItem("退出", "退出")

	// 设置菜单项的图标（可选）
	mDebug.SetIcon(icon.Data)
	mQuit.SetIcon(icon.Data)

	// 我们可以操作菜单项
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				fmt.Println("Requesting quit")
				systray.Quit()
				return
			case <-mDebug.ClickedCh:
				fmt.Println("Showing info")
				openDebugWin()
			}
		}
	}()
}
