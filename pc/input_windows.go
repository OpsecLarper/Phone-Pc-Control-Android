//go:build windows

package main

import (
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/itchyny/volume-go"
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	pSendInput   = user32.NewProc("SendInput")
	pVkKeyScanW  = user32.NewProc("VkKeyScanW")
	pSetDpiAware = user32.NewProc("SetProcessDPIAware")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mMove       = 0x0001
	mLeftDown   = 0x0002
	mLeftUp     = 0x0004
	mRightDown  = 0x0008
	mRightUp    = 0x0010
	mMiddleDown = 0x0020
	mMiddleUp   = 0x0040
	mWheel      = 0x0800

	kExtended = 0x0001
	kKeyUp    = 0x0002
	kUnicode  = 0x0004
)

// Layouts match the Windows x64 INPUT struct (40 bytes).
type mouseInput struct {
	Type      uint32
	_         uint32
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	_         uint32
	Extra     uintptr
}

type keyInput struct {
	Type  uint32
	_     uint32
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	_     uint32
	Extra uintptr
	_     [8]byte
}

func init() { pSetDpiAware.Call() }

func sendMouse(in ...mouseInput) {
	if len(in) == 0 {
		return
	}
	pSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func sendKeys(in ...keyInput) {
	if len(in) == 0 {
		return
	}
	pSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func mouseMove(dx, dy int32) {
	sendMouse(mouseInput{Type: inputMouse, Dx: dx, Dy: dy, Flags: mMove})
}

func mouseScroll(units int32) {
	// 1 unit from the phone = 1/3 of a wheel notch, for smooth scrolling
	sendMouse(mouseInput{Type: inputMouse, MouseData: uint32(units * 40), Flags: mWheel})
}

func mouseButton(b string, down bool) {
	var f uint32
	switch b {
	case "right":
		f = map[bool]uint32{true: mRightDown, false: mRightUp}[down]
	case "middle":
		f = map[bool]uint32{true: mMiddleDown, false: mMiddleUp}[down]
	default:
		f = map[bool]uint32{true: mLeftDown, false: mLeftUp}[down]
	}
	sendMouse(mouseInput{Type: inputMouse, Flags: f})
}

func typeText(s string) {
	var in []keyInput
	for _, u := range utf16.Encode([]rune(s)) {
		in = append(in,
			keyInput{Type: inputKeyboard, Scan: u, Flags: kUnicode},
			keyInput{Type: inputKeyboard, Scan: u, Flags: kUnicode | kKeyUp})
	}
	sendKeys(in...)
}

type vk struct {
	code uint16
	ext  bool
}

var namedKeys = map[string]vk{
	"backspace": {0x08, false}, "tab": {0x09, false}, "enter": {0x0D, false}, "esc": {0x1B, false},
	"space": {0x20, false}, "pageup": {0x21, true}, "pagedown": {0x22, true}, "end": {0x23, true},
	"home": {0x24, true}, "left": {0x25, true}, "up": {0x26, true}, "right": {0x27, true},
	"down": {0x28, true}, "delete": {0x2E, true}, "win": {0x5B, true},
	"next": {0xB0, true}, "prev": {0xB1, true}, "playpause": {0xB3, true},
	"f1": {0x70, false}, "f2": {0x71, false}, "f3": {0x72, false}, "f4": {0x73, false},
	"f5": {0x74, false}, "f6": {0x75, false}, "f7": {0x76, false}, "f8": {0x77, false},
	"f9": {0x78, false}, "f10": {0x79, false}, "f11": {0x7A, false}, "f12": {0x7B, false},
}

var modKeys = map[string]vk{"ctrl": {0x11, false}, "alt": {0x12, false}, "shift": {0x10, false}, "win": {0x5B, true}}

func kev(k vk, up bool) keyInput {
	var f uint32
	if k.ext {
		f |= kExtended
	}
	if up {
		f |= kKeyUp
	}
	return keyInput{Type: inputKeyboard, Vk: k.code, Flags: f}
}

func pressKey(name string, mods []string) {
	name = strings.ToLower(name)
	k, ok := namedKeys[name]
	var extraShift bool
	if !ok {
		r := []rune(name)
		if len(r) != 1 {
			return
		}
		res, _, _ := pVkKeyScanW.Call(uintptr(r[0]))
		if int16(res) == -1 {
			typeText(name)
			return
		}
		k = vk{code: uint16(res & 0xFF)}
		extraShift = res&0x100 != 0
	}
	var ms []vk
	seen := map[string]bool{}
	for _, m := range mods {
		if mk, ok := modKeys[m]; ok && !seen[m] {
			seen[m] = true
			ms = append(ms, mk)
		}
	}
	if extraShift && !seen["shift"] {
		ms = append(ms, modKeys["shift"])
	}
	var in []keyInput
	for _, m := range ms {
		in = append(in, kev(m, false))
	}
	in = append(in, kev(k, false), kev(k, true))
	for i := len(ms) - 1; i >= 0; i-- {
		in = append(in, kev(ms[i], true))
	}
	sendKeys(in...)
}

// ---- volume: all COM calls run on one locked OS thread ----

var volJobs = make(chan func(), 16)

func startVolumeWorker() {
	go func() {
		runtime.LockOSThread()
		for job := range volJobs {
			job()
		}
	}()
}

func onVolThread(f func()) {
	done := make(chan struct{})
	volJobs <- func() { f(); close(done) }
	<-done
}

func getVolume() (int, bool) {
	v, m := -1, false
	onVolThread(func() {
		if x, err := volume.GetVolume(); err == nil {
			v = x
		}
		if x, err := volume.GetMuted(); err == nil {
			m = x
		}
	})
	return v, m
}

func setVolume(v int) {
	onVolThread(func() {
		volume.SetVolume(v)
		if v > 0 {
			if m, _ := volume.GetMuted(); m {
				volume.Unmute()
			}
		}
	})
}

func setMute(on bool) {
	onVolThread(func() {
		if on {
			volume.Mute()
		} else {
			volume.Unmute()
		}
	})
}
