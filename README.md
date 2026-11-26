# ScreenControl · 屏幕管理工具

在 macOS 和 Windows 上管理多块显示器。可以给屏幕显示编号、暂停外接屏画面，或把指定显示器从桌面布局里移除并重新接回。

## 下载和安装

打开 [GitHub Releases](https://github.com/xinzhizhu510/ScreenControl/releases/latest)，按系统选择文件。

| 系统 | 文件 | 安装方式 |
| --- | --- | --- |
| macOS（Apple Silicon） | `ScreenControl-<版本>-macos-arm64.dmg` | 打开 DMG，将 `ScreenControl.app` 拖到「应用程序」。更新前退出旧版，然后替换原应用。当前构建使用临时签名，首次启动可能需要在 macOS「隐私与安全性」中确认。 |
| Windows 10/11（x64） | `ScreenControl-<版本>-windows-x64.exe` | 下载后直接运行。桌面界面使用 WebView2，缺少运行时时可从 [微软官网下载](https://developer.microsoft.com/microsoft-edge/webview2/) 安装。 |

## 操作说明

- **识别屏幕**会在已连接屏幕上显示编号。点击概览里的屏幕可跳到对应的控制项。插拔显示器后可以点「刷新」。
- **关屏 / 点亮**通过 DDC/CI 控制兼容的外接显示器。请先在显示器菜单里开启 DDC/CI。部分型号关屏后会断开控制通道，需要按实体电源键恢复。
- **断开显示器 / 重新连接**修改系统桌面布局。断开成功后，系统会重新安排该屏幕上的窗口。应用会保留至少一块屏幕，并在本次运行期间保留恢复入口。建议先接回屏幕再退出应用。
- macOS 的软断开使用未公开的系统接口，兼容性取决于系统版本、显示器和接线。无法从应用恢复时，可以重新插拔显示线。

## 本地构建

需要 `go.mod` 指定的 Go 版本、Node.js 24 和 npm。macOS 构建还需要 Apple Silicon 和 Xcode 命令行工具。

```sh
# macOS，构建应用和 DMG
./packaging/build-mac.sh
go test ./...
./packaging/build-dmg.sh
```

```sh
# 在 macOS 或 Linux 交叉构建 Windows x64
npm --prefix frontend ci
npm --prefix frontend run build
mkdir -p build
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags desktop,production -ldflags="-H windowsgui" -o build/ScreenControl-windows-x64.exe .
```

输出文件位于 `build/`。macOS 的 DDC/CI 组件来自 [waydabber/m1ddc](https://github.com/waydabber/m1ddc/tree/9d75fb6c407ac60dc77a9e32c43d60fd0e6d1612)，其许可证见 [third_party/m1ddc/LICENSE](third_party/m1ddc/LICENSE)。

## GitHub 自动构建与发布

推送 `main`、创建 Pull Request，或在 Actions 手动运行 **Build ScreenControl**，都会执行测试并生成两平台的构建产物。推送 `vX.Y.Z` 格式的标签后，两个构建任务都成功才会发布 GitHub Release，附上对应版本的 DMG 和 EXE。发布包的版本号来自标签。

```sh
git tag -a v0.2.2 -m "ScreenControl 0.2.2"
git push origin v0.2.2
```
