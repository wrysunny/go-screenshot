package main

import (
	"C"
	"fmt"
	"image"
	"image/png"
	"os"
	"syscall"
	"unsafe"
)

var (
	User32                 = syscall.NewLazyDLL("User32.dll")
	Gdi32                  = syscall.NewLazyDLL("Gdi32.dll")
	Kernel32               = syscall.NewLazyDLL("Kernel32")
	GlobalAlloc            = Kernel32.NewProc("GlobalAlloc")
	GlobalFree             = Kernel32.NewProc("GlobalFree")
	GlobalLock             = Kernel32.NewProc("GlobalLock")
	GlobalUnlock           = Kernel32.NewProc("GlobalUnlock")
	GetDC                  = User32.NewProc("GetDC")
	ReleaseDC              = User32.NewProc("ReleaseDC")
	EnumDisplayMonitors    = User32.NewProc("EnumDisplayMonitors")
	GetDesktopWindow       = User32.NewProc("GetDesktopWindow")
	DeleteDC               = Gdi32.NewProc("DeleteDC")
	DeleteObject           = Gdi32.NewProc("DeleteObject")
	GetDeviceCaps          = Gdi32.NewProc("GetDeviceCaps")
	SelectObject           = Gdi32.NewProc("SelectObject")
	BitBlt                 = Gdi32.NewProc("BitBlt")
	GetDIBits              = Gdi32.NewProc("GetDIBits")
	CreateCompatibleDC     = Gdi32.NewProc("CreateCompatibleDC")
	CreateCompatibleBitmap = Gdi32.NewProc("CreateCompatibleBitmap")
	/*
		funcGetDesktopWindow, _    = syscall.GetProcAddress(syscall.Handle(libUser32), "GetDesktopWindow")
		funcEnumDisplayMonitors, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "EnumDisplayMonitors")
		funcGetMonitorInfo, _      = syscall.GetProcAddress(syscall.Handle(libUser32), "GetMonitorInfoW")
		funcEnumDisplaySettings, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "EnumDisplaySettingsW")
	*/
)

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

// 单个显示器 获取分辨率
func GetScreenResolution() image.Rectangle {
	HDC, _, _ := GetDC.Call(uintptr(0))

	defer ReleaseDC.Call(uintptr(0), HDC)

	width, _, _ := GetDeviceCaps.Call(HDC, uintptr(118))
	height, _, _ := GetDeviceCaps.Call(HDC, uintptr(117))
	rect := image.Rect(0, 0, int(width), int(height))
	return rect
}

// 获取存在几个显示器
func ActiveDisplaysNum() int {
	HDC, _, _ := GetDC.Call(uintptr(0))

	defer ReleaseDC.Call(uintptr(0), HDC)

	var count int = 0
	// https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-enumdisplaymonitors
	EnumDisplayMonitors.Call(
		HDC,
		uintptr(0),
		syscall.NewCallback(
			func(hMonitor, hdcMonitor, lprcMonitor, dwData uintptr) uintptr {
				var count *int
				count = (*int)(unsafe.Pointer(dwData))
				*count = *count + 1
				return uintptr(1)
			}),
		uintptr(unsafe.Pointer(&count)),
	)
	return count
}

// 截图返回图片RGBA
func Capture(rect image.Rectangle) *image.RGBA {
	hwnd, _, _ := GetDesktopWindow.Call(uintptr(0))
	HDC, _, _ := GetDC.Call(hwnd)

	defer ReleaseDC.Call(hwnd, HDC)

	memory_device, _, _ := CreateCompatibleDC.Call(HDC)

	defer DeleteDC.Call(memory_device)

	img := image.NewRGBA(rect)
	bitmap, _, _ := CreateCompatibleBitmap.Call(HDC, uintptr(rect.Dx()), uintptr(rect.Dy()))

	defer DeleteObject.Call(bitmap)

	var header BITMAPINFOHEADER
	header.BiSize = uint32(unsafe.Sizeof(header))
	header.BiPlanes = 1
	header.BiBitCount = 32
	header.BiWidth = int32(rect.Dx())
	header.BiHeight = int32(-rect.Dy())
	header.BiCompression = 0
	header.BiSizeImage = 0
	// https://docs.microsoft.com/en-gb/windows/desktop/gdi/capturing-an-image
	bitmapDataSize := uintptr(((int64(rect.Dx())*int64(header.BiBitCount) + 31) / 32) * 4 * int64(rect.Dy()))
	hmem, _, _ := GlobalAlloc.Call(0x0002, bitmapDataSize)

	defer GlobalFree.Call(hmem)

	memptr, _, _ := GlobalLock.Call(hmem)

	defer GlobalUnlock.Call(hmem)

	old, _, _ := SelectObject.Call(memory_device, bitmap)
	if old == 0 {
		fmt.Println("SelectObject failed")
		return nil
	}

	defer SelectObject.Call(memory_device, old)

	f, _, _ := BitBlt.Call(memory_device, 0, 0, uintptr(rect.Dx()), uintptr(rect.Dy()), HDC, uintptr(0), uintptr(0), uintptr(0x00CC0020))
	if !(f != 0) {
		fmt.Println("BitBlt failed")
		return nil
	}
	v, _, _ := GetDIBits.Call(HDC, bitmap, 0, uintptr(rect.Dy()), memptr, uintptr(unsafe.Pointer(&header)), uintptr(0))
	if int32(v) == 0 {
		fmt.Println("GetDIBits failed")
		return nil
	}
	//
	i := 0
	src := memptr
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			v0 := *(*uint8)(unsafe.Pointer(src))
			v1 := *(*uint8)(unsafe.Pointer(src + 1))
			v2 := *(*uint8)(unsafe.Pointer(src + 2))

			// BGRA => RGBA, and set A to 255
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v2, v1, v0, 255

			i += 4
			src += 4
		}
	}
	return img
}

//export screenshot
func screenshot() {
	// 判断存在多个显示器
	if ActiveDisplaysNum() > 1 {
		// 多个显示器暂不考虑
		return
	} else if ActiveDisplaysNum() == 1 {
		// 单个显示器 获取分辨率
		rect := GetScreenResolution()
		// 截图
		img := Capture(rect)

		defer func() {
			syscall.FreeLibrary(syscall.Handle(User32.Handle()))
			syscall.FreeLibrary(syscall.Handle(Gdi32.Handle()))
			syscall.FreeLibrary(syscall.Handle(Kernel32.Handle()))
		}()

		file, _ := os.Create("screen.png")
		defer file.Close()
		png.Encode(file, img)

	}
}

func main() {
	screenshot()
}
