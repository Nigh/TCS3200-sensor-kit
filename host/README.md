# Linux 上位机

MyGo 原生 Go UI，展示 TCS3200 的 R/G/B/Clear 四通道频率；使用 [USB 协议 v1](../docs/protocol.md)。不使用浏览器前端。首版仅测量，不提供测试配置、键鼠触发、BOOTSEL、导出或超过 10 秒的历史。

## 运行与构建

需要 Go **1.27.1+**（`GOTOOLCHAIN=auto` 可自动下载），以及可用的 Linux 图形桌面。原生界面需要 GTK 3 与 OpenGL 驱动；无需 WebKitGTK 或 Node/Bun。Ubuntu/Debian 可安装 `libgtk-3-0`、`libgl1`、`fonts-noto-cjk`；其他发行版安装对应运行库。工具链和 Go 依赖仅首次构建时需要联网。

在仓库根目录运行：

```sh
make host          # 启动实际设备界面
make host-demo     # 合成 1 kHz 信号，明确标注，禁用 USB 连接；无需硬件
make host-build    # 发布构建：host/build/tcs3200-host（CGO_ENABLED=0）
make host-test     # Go 测试与 vet
```

也可在本目录运行 `go run .`、`go run . --demo`。现有根目录 `make`、`make test` 仍用于固件。

窗口默认 1280×900，最小 1080×760。使用 Noto Sans CJK SC 和框架字体回退。Wayland 与 X11 由 GTK 选择；若特定驱动下 Wayland 异常，可用 `GDK_BACKEND=x11 ./host/build/tcs3200-host` 在 XWayland 中运行。

## 设备连接

1. 点击“枚举设备”，只列出 VID `ACDC` / PID `4015` 的 USB CDC 端口，以端口路径和序列号区分多块板。
2. 选择设备并点击“连接”。应用打开二进制 CDC、置 DTR，持续读取，同时通过 GET_STATUS 检查协议布局及 1000 Hz / 250 μs 采样参数。波特率设置为 115200，不限制 USB bulk 传输速度。
3. 状态显示“已连接”后观察四通道波形。意外拔出、接收错误或连续 2 秒没有有效采样时停止连接并保留最后波形，显示错误原因。重新插入后手动枚举、选择、连接；不会自动重连。
4. “断开”关闭 CDC 并取消未完成命令。新连接清空旧解码、时间轴、原始数据和快照。

**权限错误：**确认端口存在且未被串口终端占用，检查 `ls -l /dev/ttyACM*` 的属组。按发行版规则将用户加入相应串口组（Ubuntu 通常为 `dialout`），重新登录后再启动。不要使用 root 启动 GUI，也不需要修改设备为所有人可写。枚举失败会显示错误，空列表会提示检查 USB。

## 图表和统计

- 四图共享 X 轴；默认最近 10 秒，Y 轴分别自动取整。所有读数为 **kHz**：协议的一个计数为 10 Hz，因此显示值为原始值除以 100。
- 使用设备采样时间而非 USB 到达时间绘图；展开 32 位时间戳回绕。序号缺失或时间不连续的位置断开折线，保留 LOW/CLIP/LATE 标志。LOW 的原始零值仍参与统计；CLIP 表示饱和，读数不是精确频率。
- 滚轮以指针位置缩放 X 轴，范围 **10 ms–10 s**；开始缩放或拖动即固定快照。拖动在快照内平移；图表取得键盘焦点后，`=` / `+`、`-` 缩放，左右方向键平移。
- “暂停刷新”冻结当前最多 10 秒的原始样本，USB 继续接收并独立更新最近 10 秒缓冲。“恢复实时”“回到实时”或 Esc 放弃旧快照，显示最新 10 秒。长时间暂停不会累计无限历史。
- 悬停显示当前物理像素时间桶的区间、样本数、Max、Min、中位数，并联动四图时间游标。统计来自原始样本，和绘制降采样分离；偶数样本的中位数取中间两数平均。空桶显示无数据。时间区间以当前视口右端为 0；最后一个桶包含右端样本。
- 底部显示主机按序号计算的丢组、设备采集/发送丢组和两端协议错误。设备计数从启动累计，主机计数从连接会话开始；暂停不停止采集或命令处理。图表采集刷新请求最多 30 Hz，鼠标和键盘交互可额外触发即时帧。

## 低频阈值

输入 **10000–600000 的整数 Hz**，点击“应用”。仅测量模式且设备处于 WAIT 时可提交。成功后等待 ACK、GET_STATUS 回读一致，再清空旧图表和快照；按设备时钟丢弃直到状态快照之后 100 ms 的样本，避免把排队旧采样混入新阈值图表。

阈值设置会按固件约定使已有测试配置失效，配置只存于设备 RAM。设备拒绝命令时保留连接并显示结果；通信超时后不自动重发，而是断开并提示重新连接确认。命令严格串行，默认响应超时 1 秒。

## 临时性能优化与后续迁移

参考 [mygo PR #363](https://github.com/egoist/mygo/pull/363)，当前只在应用侧按**物理像素列**取 Min/Max，按原始时间顺序保留极值、连续段端点与缺口；复用输出缓冲，正常连续波形绘制点数约为每像素两个点。原始 1000 Hz 数据仍保留，未做平滑或平均。

依赖固定到上游 `7769928c78b7`（完整版本见 go.mod），没有使用相邻 mygo checkout、replace 或未合并 fork。临时代码在 `data.go`，带 `shortcut:` 注释。目前仍使用 `chart.Line` / `StrokePath` CPU 路径，不能承诺 PR GPU 路径的性能。

PR 合并并发布后：

1. 显式升级并固定包含 PR 的上游版本，核对公开 API。
2. 用 `chart.MinMax` 替换本地 `minmax`，用 `Painter.StrokePolyline` 替换当前 `chart.Line` / `StrokePath` 调用，复用折线点缓冲。
3. 删除临时降采样实现及其专用测试；保留原始样本、hover、缺口和生命周期测试。
4. 重跑 Linux 1×/2× 渲染、交互、CPU/内存与持续运行检查。仅升级依赖不会自动切换到 PR 的 opt-in GPU API。

主题参考 [Nigh/xianii-theme](https://github.com/Nigh/xianii-theme)（MIT）；在 `ui.Theme` 中映射其中性深色、粉色强调与 8 DIP 圆角，没有引入 CSS/Svelte 依赖。

## 验证

```sh
cd host
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -tags mygo_noinspector ./...
go test -run '^$' -bench BenchmarkMinMax -benchmem
TCS3200_SCREENSHOTS=build/screens go test -run 'TestMeasurementUI|TestEmptyUI'
```

测试覆盖协议校验向量、任意拆包/粘包、坏 CRC 和截断恢复、时间与序号回绕、三分钟等量合成采样的有界保留、丢组、Min/Max 尖峰和缺口、统计、CDC 打开失败/取消/拒绝/超时/关闭/重连，以及原生界面的 hover、暂停、缩放、平移和恢复。截图检查使用框架原生 headless 渲染；另在当前 Linux 桌面启动过实际窗口。

这些测试不代替 USB 实机验收。需接入板卡确认：多设备选择、DTR 启流、连续采集、阈值变化、忙状态、实际拔插与恢复、真实 LOW/CLIP/LATE、长时间运行及 1×/2× GPU 表现。合成采样的时间跨度测试不是三分钟硬件稳定性测试。
