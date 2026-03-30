// +build linux

package watcher

import (
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// FanotifyCapability 表示 fanotify 的能力支持情况
type FanotifyCapability struct {
	// Supported 表示是否支持 fanotify
	Supported bool
	// Reason 表示不支持的原因
	Reason string
	// KernelVersion 是内核版本
	KernelVersion string
	// HasPermissions 表示是否有权限使用
	HasPermissions bool
}

// CheckFanotifySupport 检测系统是否支持 fanotify
func CheckFanotifySupport() FanotifyCapability {
	cap := FanotifyCapability{}
	
	// 1. 检测内核版本
	uname := &syscall.Utsname{}
	if err := syscall.Uname(uname); err != nil {
		cap.Reason = "无法获取内核版本"
		return cap
	}
	
	// 转换内核版本字符串
	version := charsToString(uname.Release[:])
	cap.KernelVersion = version
	
	// 解析内核版本（例如：5.15.0-generic）
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		cap.Reason = "无法解析内核版本: " + version
		return cap
	}
	
	var major, minor int
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)
	
	// fanotify 需要 Linux 5.1+
	if major < 5 || (major == 5 && minor < 1) {
		cap.Reason = fmt.Sprintf("内核版本过低: %s (需要 >= 5.1)", version)
		return cap
	}
	
	// 2. 检测是否有权限（需要 root 或 CAP_SYS_ADMIN）
	// 尝试初始化 fanotify
	fd, err := unix.FanotifyInit(0, 0)
	if err != nil {
		if err == syscall.EPERM {
			cap.Reason = "权限不足（需要 root 或 CAP_SYS_ADMIN）"
			return cap
		}
		cap.Reason = "fanotify 初始化失败: " + err.Error()
		return cap
	}
	unix.Close(fd)
	
	// 3. 检测是否支持 FAN_REPORT_DIR_FID（需要 5.1+）
	fd, err = unix.FanotifyInit(unix.FAN_REPORT_DIR_FID, 0)
	if err != nil {
		cap.Reason = "不支持 FAN_REPORT_DIR_FID（需要内核 >= 5.1）"
		return cap
	}
	unix.Close(fd)
	
	cap.Supported = true
	cap.HasPermissions = true
	return cap
}

// charsToString 将 C 字符数组转换为 Go 字符串
func charsToString(ca []int8) string {
	s := make([]byte, len(ca))
	var l int
	for ; l < len(ca); l++ {
		if ca[l] == 0 {
			break
		}
		s[l] = uint8(ca[l])
	}
	return string(s[:l])
}

// GetWatcherType returns the type of watcher being used.
// Now it returns "fanotify" or "fsnotify" based on support.
func GetWatcherType() string {
	cap := CheckFanotifySupport()
	if cap.Supported {
		return "fanotify"
	}
	return "fsnotify"
}

// PrintFanotifyWarning prints detailed warning about using fsnotify instead of fanotify.
// This should be called after all directories are watched.
func PrintFanotifyWarning() {
	cap := CheckFanotifySupport()
	if !cap.Supported {
		// 简单的 fallback 提醒
		log.Printf("WARNING: Using fsnotify (inotify) as fallback (fanotify not available)")
		// 详细的 sudo/cap_sys_admin 提示
		log.Printf("WARNING: Using fsnotify (inotify) instead of fanotify. Reason: %s", cap.Reason)
		log.Printf("WARNING: To use fanotify for better performance:")
		log.Printf("WARNING:   sudo setcap cap_sys_admin+ep ./bin/golocated")
		log.Printf("WARNING:   or run as root: sudo ./bin/golocated --service")
	}
}

// IsFanotifySupported returns true if fanotify is supported and available.
func IsFanotifySupported() bool {
	cap := CheckFanotifySupport()
	return cap.Supported
}

// GetFanotifyCapability returns detailed fanotify capability info.
func GetFanotifyCapability() FanotifyCapability {
	return CheckFanotifySupport()
}

// canUseFanotify 检查是否可以使用 fanotify
// 这是一个简化的检查，用于快速判断
func canUseFanotify() bool {
	// 快速检查：是否是 root 用户
	if os.Geteuid() == 0 {
		return true
	}
	
	// 非 root 用户，检查是否有 CAP_SYS_ADMIN
	// 简化处理：非 root 用户尝试初始化
	fd, err := unix.FanotifyInit(0, 0)
	if err != nil {
		return false
	}
	unix.Close(fd)
	return true
}
