# Clipboard Sync

一个通过 Debian TCP 中转端，让 Android 与 Android、Windows 与 Android 等客户端双向同步文本剪贴板的项目：

- `cmd/relay`：Debian/Linux TCP 中转端，默认监听 `0.0.0.0:65432`，维护客户端长连接并将更新转发给其他客户端。
- `cmd/windows-client`：Windows 文本剪贴板客户端，轮询本机剪贴板并保持 TCP 长连接。
- `android`：Android 客户端，`minSdk 21`，使用前台服务保持 TCP 长连接。
- `.github/workflows/build.yml`：GitHub Actions 自动测试和构建 Linux amd64/arm64 中转端、Windows amd64/arm64 客户端及 Android APK。

## 网络与安全说明

此版本使用 TCP 长连接，但**不提供加密、身份认证或访问控制**。能够访问中转端的设备可以伪造剪贴板更新；剪贴板文本也会以明文在网络上传输。请只在自己控制、可信任的网络使用，不要将 TCP 端口直接暴露到公网。

所有客户端（包括不同网络中的客户端）都只需要能够连接 Debian 中转端的 TCP/65432 端口；防火墙和云安全组需允许该端口入站。中转端按连接转发给其他客户端，不把内容保存为历史记录；离线期间发生的剪贴板变更不会补发。同步的是纯文本，不包括图片或富文本。

Android 10 及以上系统只允许前台应用或默认输入法读取剪贴板；前台服务不能绕过此系统限制。因此 Android 设备向其他端发送本机剪贴板更新时，需要让本应用保持在前台，或将它实现为默认输入法；Android 接收 TCP 更新并写入剪贴板不受此读取限制影响。Android 应用本身是架构无关的 APK，不区分 32/64 位手机 ABI，GitHub 工作流产出的 APK 可安装在支持的 32 位和 64 位 Android 设备上。

## Debian/Linux 中转端

需要 Go 1.22 或更高版本。构建并启动：

```sh
go build -o clipboard-relay ./cmd/relay
sudo ./clipboard-relay -listen :65432
```

在服务器防火墙开放 TCP/65432。客户端连接中转端，不需要同处一个广播域。

GitHub Actions 的 `Build` 工作流会发布独立的 `clipboard-relay-linux-amd64` 和 `clipboard-relay-linux-arm64` 构建产物。

## Windows 客户端

在客户端电脑运行，填入 Debian 中转端的局域网 IP：

```powershell
.\clipboard-client-windows-amd64.exe -server 192.168.1.20
```

可用 `-port 65432` 和 `-poll 500ms` 调整中转端端口及剪贴板检测间隔。Windows 客户端主动连接服务器，不需要开放客户端入站端口。程序启动时只记录本机剪贴板初始值，不会主动发送启动前已有的内容；之后发生变化的文本会同步。

GitHub Actions 同时提供 Windows amd64 和 arm64 客户端。

## Android 客户端

从 GitHub Actions 下载 `clipboard-sync-android-debug` 构建产物中的 `app-debug.apk` 并安装。打开应用，输入 Debian 中转端 IP/主机名，点击“启动同步”，并允许通知权限。前台服务通知显示连接和同步状态；点击“停止同步”结束服务。要让 Android 设备之间同步，每台设备都安装 APK 并连接到同一中转端。

## 本地验证

```sh
go test ./...
GOOS=windows GOARCH=amd64 go build -o clipboard-client.exe ./cmd/windows-client
```

Android 可使用已安装的 Gradle 8.9 和 Android SDK 构建：

```sh
gradle -p android assembleDebug
```
