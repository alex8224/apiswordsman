package nativejs

import (
	"fmt"
	"time"

	"github.com/dop251/goja"
)

type Console struct {
}

type NativeFuncCtx struct {
	vm      *goja.Runtime
	console *Console
}

func (console *Console) Log(msg string) {
	fmt.Println(msg)
}

func NewCtx(vm *goja.Runtime) *NativeFuncCtx {
	con := &Console{}
	return &NativeFuncCtx{vm: vm, console: con}
}

func (ctx *NativeFuncCtx) Log(key string) {
	ctx.console.Log(key)
}

func (ctx *NativeFuncCtx) Add(a, b int64) int64 {
	return a + b
}

func (ctx *NativeFuncCtx) Sleep(mills int64) {
	time.Sleep(time.Duration(mills) * time.Millisecond)
}
