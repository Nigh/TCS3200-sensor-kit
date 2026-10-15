# TCS3200 Sensor Kit

RP2040-Zero 测量屏幕颜色：PIO 测周期，RGB+C 每毫秒一组，CDC bulk 传输；同一固件可配置 ARM 颜色，偏离后延迟发送键盘或鼠标按钮。

| 目录 | 内容 |
|---|---|
| `firmware/` | Pico SDK + TinyUSB 固件、PIO、测试与接线说明 |
| `host/` | MyGo 原生 Linux 上位机、实时四通道图表与 USB CDC 连接 |
| `docs/protocol.md` | 两端共用的 USB 二进制协议、字段单位与状态语义 |

基于 [Nigh/pico-cdc-hid-template](https://github.com/Nigh/pico-cdc-hid-template) 建立，保留 MIT 许可。已移除文本日志、通用 HID 回显及 LED 调度；HID 为键盘与鼠标。

在仓库根目录运行：

```sh
make       # 使用 xianii/pico-sdk:2.3.0 Docker 镜像编译
make test  # 主机 C 自检（ASan/UBSan）及 PIO 时序模拟；需要 cc 和 Python 3
```

输出：`firmware/build/src/tcs3200_sensor.uf2`。按住 BOOT 接入 USB，复制 UF2 到引导盘。详细接线与验证见 [固件说明](firmware/README.md)，上位机开发以 [协议 v1](docs/protocol.md) 为准。

软件自检与交叉编译不代替硬件验证。1000 Hz 是计划组频率，低频/超时通道置零；屏幕 PWM、刷新与各通道时间偏移可能影响颜色判定。

## Linux 上位机

```sh
make host          # 实际设备界面；手动枚举、选择、连接
make host-demo     # 无硬件合成信号演示
make host-build    # host/build/tcs3200-host
make host-test     # 上位机测试和 vet
```

需要 Go 1.27.1+、GTK 3 与 OpenGL。支持四通道最近 10 秒、X 轴缩放、暂停快照、像素桶 Max/Min/中位数及低频阈值设置。详见 [上位机说明](host/README.md)，包括串口权限、临时 Min/Max 优化和后续上游迁移。
