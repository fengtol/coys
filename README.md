# Clipboard Sync

一个面向可信任局域网的文本剪贴板同步示例项目：

- `cmd/relay`：Debian/Linux UDP 中转端，默认监听 `0.0.0.0:65432`，并将合法更新广播到 `255.255.255.255:65432`。
- `cmd/windows-client`：Windows 文本剪贴板客户端，轮询本机剪贴板并监听中转端广播。
- `android`：Android 客户端，`minSdk 21`，使用前台服务保持 UDP 接收。
- `.github/workflows/build.yml`：GitHub Actions 自动测试和构建 Linux amd64/arm64 中转端、Windows amd64/arm64 客户端及 Android APK。

## 网络与安全说明

此版本按 UDP 局域网广播设计，**不提供加密、身份认证或访问控制**。同一网络内能够访问中转端的设备可以伪造剪贴板更新；剪贴板文本也会以明文在网络上传输。请只在自己控制、可信任的局域网使用，不要将 UDP 端口直接暴露到公网。

IP 广播通常不会跨路由器/VLAN。Windows/Android 客户端需能访问 Debian 中转端，且中转端广播地址应属于客户端所在的广播域。如果 Debian 有多个网卡，建议显式指定正确的子网广播地址。客户端和服务端使用本地 UDP/65432 端口，确保该端口未被其他程序占用。UDP 不保证送达；本项目同步的是纯文本，不包括图片或富文本。

Android 10 及以上系统可能限制后台应用读取剪贴板。前台服务会尝试监听和轮询，但具体后台读取行为由 Android 版本及厂商系统决定；在应用前台时同步更可靠。Android 应用本身是架构无关的 APK，不区分 32/64 位手机 ABI，GitHub 工作流产出的 APK 可安装在支持的 32 位和 64 位 Android 设备上。

## Debian/Linux 中转端

需要 Go 1.22 或更高版本。构建并启动：

```sh
go build -o clipboard-relay ./cmd/relay
sudo ./clipboard-relay -listen :65432 -broadcast 192.168.1.255:65432
```

将 `192.168.1.255` 替换为 Debian 所在子网的广播地址。只有单一简单局域网时可省略 `-broadcast`，使用默认 `255.255.255.255:65432`。防火墙需要允许客户端向 UDP/65432 入站，以及中转端向子网 UDP/65432 出站。

GitHub Actions 的 `Build` 工作流会发布独立的 `clipboard-relay-linux-amd64` 和 `clipboard-relay-linux-arm64` 构建产物。

## Windows 客户端

在客户端电脑运行，填入 Debian 中转端的局域网 IP：

```powershell
.\clipboard-client-windows-amd64.exe -server 192.168.1.20
```

可用 `-poll 500ms` 调整剪贴板检测间隔。客户端接收本机 UDP/65432 上的广播，因此确保本机防火墙允许该端口。程序启动时只记录剪贴板初始值，不会主动发送启动前已有的内容；之后发生变化的文本会同步。

GitHub Actions 同时提供 Windows amd64 和 arm64 客户端。

## Android 客户端

从 GitHub Actions 下载 `clipboard-sync-android-debug` 构建产物中的 `app-debug.apk` 并安装。打开应用，输入 Debian 中转端局域网 IP，点击“启动同步”，并允许通知权限。前台服务通知显示同步状态；点击“停止同步”结束服务。

## 本地验证

```sh
go test ./...
GOOS=windows GOARCH=amd64 go build -o clipboard-client.exe ./cmd/windows-client
```

Android 可使用已安装的 Gradle 8.9 和 Android SDK 构建：

```sh
gradle -p android assembleDebug
```
