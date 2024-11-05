package main

type tray struct {
}

func (tray *tray) RunTray(onReady func(), onExit func()) {
	defer onExit()
	onReady()
}

func MakeTray() *tray {
	return &tray{}
}
