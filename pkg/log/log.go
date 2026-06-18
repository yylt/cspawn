package log

import (
	"fmt"
	"os"
	"strings"
)

var (
	DebugEnabled = os.Getenv("CSPAWN_DEBUG") == "1"
)

func Debug(format string, args ...interface{}) {
	if DebugEnabled {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

func Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
}

func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}

func ImagePulling(image string) {
	Info("Pulling image / 拉取镜像: %s", image)
}

func ImagePulled(image string) {
	Info("Image pulled / 镜像拉取完成: %s", image)
}

func RootfsPreparing(path string) {
	Info("Preparing rootfs / 准备 rootfs: %s", path)
}

func RootfsReady(path string) {
	Info("Rootfs ready / rootfs 就绪: %s", path)
}

func MountAdding(dest string) {
	Debug("Adding mount / 添加挂载: %s", dest)
}

func MountExists(dest string) {
	Debug("Mount exists, skipping / 挂载已存在，跳过: %s", dest)
}

func DeviceCreating(path string) {
	Debug("Creating device / 创建设备: %s", path)
}

func DeviceExists(path string) {
	Debug("Device exists, skipping / 设备已存在，跳过: %s", path)
}

func ContainerStarting(command []string) {
	Info("Starting container / 启动容器: %s", strings.Join(command, " "))
}

func ContainerExec(command []string) {
	Info("Executing command / 执行命令: %s", strings.Join(command, " "))
}
