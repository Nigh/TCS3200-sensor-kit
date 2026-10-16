# TCS3200 Sensor Kit USB 协议 v2

本文件是固件与上位机共同遵守的协议定义。固件实现位于 `firmware/src/protocol.h`、`protocol.c`、`usb_func.c`；上位机实现必须以本文为准。更改字段、单位、枚举或行为时，必须同时更新本文及测试；不兼容更改须提升帧版本。本文描述已经实现的 v2 行为。

v2 保持帧和 Payload 的字段布局不变，但改变了固定 1 kHz/250 μs 的采样语义并放宽阈值；Version 从 01 升为 02。固件与上位机须配套升级，旧 v1 客户端/固件不兼容。

## USB 与连接

- 设备：RP2040-Zero，USB Full Speed，VID `0xACDC`、PID `0x4015`，序列号为板载 Flash 唯一 ID。PID 与原模板不同，避免通用 HID 的缓存绑定。
- CDC ACM：数据 IN `0x85`、OUT `0x04`，bulk，最大包长 64 字节；通知端点 `0x84`。没有文本日志、换行命令或串口回显。
- 上位机打开 CDC 后必须置 DTR=1，持续读取数据。波特率设置不影响 USB 数据速率；建议使用原始二进制串口模式。
- HID：IN `0x81`，interrupt，轮询间隔 1 ms；一个 HID 接口包含键盘 Report ID=1 与鼠标 Report ID=2。鼠标动作仅发送按钮，位移和滚轮为零；键盘支持一个普通键及修饰键组合。
- 默认测量模式。两种模式均持续发送采样；切换模式不重新枚举 USB。
- CDC 的一次读取、USB 包与协议帧没有一一对应关系。上位机必须处理任意拆包和粘包。
- 关闭/重开串口时清空上位机解码缓冲；USB 挂起或断连可能截断尚未完成的帧，恢复后按 Magic/CRC 重新同步。
- 同时最多一个未完成命令；响应可以穿插在采样帧之间。没有自动重试或命令去重，不能盲目重发成功过的模式命令。

## 帧格式

所有多字节整数都是**无符号、小端**。不得直接序列化有内存填充的 C 结构体。

| 偏移 | 字节数 | 字段 |
|---:|---:|---|
| 0 | 2 | Magic：`54 43`，ASCII `TC` |
| 2 | 1 | Version：`02` |
| 3 | 1 | Type |
| 4 | 2 | Sequence |
| 6 | 2 | Payload length：0～64 |
| 8 | N | Payload |
| 8+N | 2 | CRC-16/CCITT-FALSE，小端 |

帧长为 `10+N`。CRC 覆盖偏移 2 到 `7+N`（Version 至 Payload 末尾），不包含 Magic 和 CRC；多项式 `0x1021`、初始值 `0xFFFF`、不反射、最终异或 `0x0000`。标准校验串 `123456789` 的 CRC 为 `0x29B1`。

命令序号由上位机分配，响应原样回传；采样帧的 Sequence 是 Payload 中 32 位采样序号的低 16 位，两者不是同一个命令序号空间。所有计数按自身位宽回绕。

接收方搜索 Magic，验证版本、长度与 CRC。错误后从候选帧起点向后移动一个字节继续搜索。固件对不完整候选帧使用 100 ms 的字节间超时，过期后丢弃。错误 CRC、非法帧版本或超过 64 字节的长度不回复，累计到 `parser_errors`。合法帧内的未知命令或错误参数返回 ACK。

## 命令与响应

| Type | 方向 | Payload | 成功响应 |
|---|---|---|---|
| `01 GET_STATUS` | 上位机→设备 | 空 | `81 STATUS` |
| `02 SET_MODE` | 上位机→设备 | 1 字节 mode | `80 ACK` |
| `03 SET_CONFIG` | 上位机→设备 | 28 字节测试配置 | `80 ACK` |
| `04 GET_CONFIG` | 上位机→设备 | 空 | `84 CONFIG` |
| `05 BOOTSEL` | 上位机→设备 | 空 | `80 ACK`，约 250 ms 后进入 UF2 引导 |
| `06 SET_LOW` | 上位机→设备 | u32 low_hz | `80 ACK` |
| `40 SAMPLE` | 设备→上位机 | 22 字节采样 | 无 |

`SET_MODE`：0=测量，1=测试。未配置不能进入测试模式。设置模式会重置 ARM/确认/尚未发送的延迟动作；测量模式取消动作并释放已发送的按键。按下报告待完成、保持或释放期间请求测试模式返回 BUSY，且不改变当前状态。测试模式只接受模式命令之后开始采集的样本。

`SET_CONFIG`：完整验证通过后一次性更新，自动返回测量模式；取消待执行动作，已按下的旧动作仍按旧配置完成释放。配置只在 RAM 中保存，断电丢失。所有测试参数应由测量实测后填写，没有默认 ARM 颜色或默认按键。

`SET_LOW`：用于尚未填写测试参数时调整测量低频阈值；范围 4000～600000 Hz，仅测量模式且 WAIT 状态接受。成功后清除“测试配置有效”标志，需要重新 SET_CONFIG 才能测试。阈值在采样核下一组开始时生效；CDC 中已经排队的旧采样不清除，上位机调整后应丢弃缓存的旧数据，再采集参考色。

`BOOTSEL`：仅测量模式且 WAIT 状态接受；等待释放完成后才可使用。重启期间其他命令返回 BUSY。ACK 尽力发送，主机停止读取时不能保证重启前收到它。拔掉 USB 会取消尚未执行的重启。

### ACK，Type=80，Payload 长度=2

| 偏移 | 类型 | 字段 |
|---:|---|---|
| 0 | u8 | 原命令 Type |
| 1 | u8 | result |

result：0=OK，1=长度错误，2=参数值错误，3=尚未配置测试，4=BUSY，5=未知命令。GET_STATUS/GET_CONFIG 成功时直接返回对应数据帧，不再发送 ACK；失败时发送 ACK。

### STATUS，Type=81，Payload 长度=32

| 偏移 | 类型 | 字段 |
|---:|---|---|
| 0 | u16 | sample_rate_hz，当前阈值对应的最大计划组频率，240～1000 Hz（向下取整） |
| 2 | u16 | channel_slot_us，当前阈值对应的通道时间槽，250～1040 μs |
| 4 | u32 | 当前 low_hz，默认 15000 |
| 8 | u32 | capture_count，已完成采集的组数 |
| 12 | u32 | capture_drops，采集队列满丢组及调度超时跳过的组数 |
| 16 | u32 | stream_drops，连接中因发送缓冲满丢弃的采样组数 |
| 20 | u32 | parser_errors，候选帧版本/长度/CRC/超时错误数 |
| 24 | u32 | uptime_ms，设备启动后的毫秒数 |
| 28 | u8 | mode |
| 29 | u8 | state |
| 30 | u8 | configured，0/1 |
| 31 | u8 | 保留，发送 0 |

计数从启动累计；断连时未发送的缓冲数据清空，但不计入 stream_drops。STATUS 是各字段的顺序读取，不保证跨核计数来自同一个时刻。

### 测试配置，SET_CONFIG Payload 长度=28

| 偏移 | 类型 | 字段及约束 |
|---:|---|---|
| 0 | u32 | low_hz，4000～600000 Hz |
| 4 | 3×u16 | ARM 参考 R/G/B，单位 10 Hz，每项 0～65534 |
| 10 | u16 | enter_tolerance，进入容差 |
| 12 | u16 | exit_threshold，必须大于 enter_tolerance |
| 14 | u16 | arm_ms，连续匹配时间，1～60000 ms |
| 16 | u16 | depart_ms，连续偏离时间，1～60000 ms |
| 18 | u32 | delay_ms，从确认偏离到提交按下报告，0～60000 ms |
| 22 | u16 | hold_ms，从按下报告完成到提交释放报告，1～60000 ms |
| 24 | u8 | action，1=键盘，2=鼠标 |
| 25 | u8 | modifiers，键盘修饰位；鼠标必须为 0 |
| 26 | u8 | keycode，键盘 HID Usage 0x04～0xDF；鼠标必须为 0 |
| 27 | u8 | mouse_buttons，鼠标 1～31；键盘必须为 0 |

容差与退出阈值均使用和采样一致的 10 Hz 单位。参考色直接来自当前安装位置与屏幕亮度下的传感器采样，不是屏幕 sRGB，也不进行自动归一化；上位机应依据实际波动设置容差。不能从“16 位传输”推断传感器有 16 位有效精度。

modifiers bit0～7：左 Ctrl、左 Shift、左 Alt、左 GUI、右 Ctrl、右 Shift、右 Alt、右 GUI。mouse_buttons bit0～4：左、右、中、后退、前进。keycode 是键位 Usage，不是字符编码，例如 `0x04` 为 A 键；字符结果由系统键盘布局和修饰键决定。

CONFIG 响应 Type=84 的 Payload 长度=29：偏移 0 是 configured（0/1），偏移 1～28 是上述配置。configured=0 时只有 low_hz 可用于测量，其余字段不构成可执行的测试配置。

## SAMPLE，Type=40，Payload 长度=22

| 偏移 | 类型 | 字段 |
|---:|---|---|
| 0 | u32 | sample_sequence，每个计划采样组递增，丢组不回补 |
| 4 | u32 | timestamp_us，设备启动后的计划组起始时间 |
| 8 | 4×u16 | R、G、B、Clear，单位 10 Hz |
| 16 | u8 | low_mask，bit0～3 对应 R/G/B/C |
| 17 | u8 | clip_mask，bit0～3 对应 R/G/B/C |
| 18 | u8 | mode，处理本组时的当前模式 |
| 19 | u8 | state，处理本组后的触发状态 |
| 20 | u8 | flags，bit0=LATE，其余为 0 |
| 21 | u8 | 保留，发送 0 |

一帧恰好 32 字节，1000 组/秒约 32 KB/s，另加少量命令响应。序号和时间戳在采集端生成；上位机通过 32 位序号差值识别丢组，通过时间戳绘图，不能用 USB 到达时间代替采样时间。timestamp_us 约每 71.6 分钟回绕，上位机按模 2^32 差值展开时间。

### 采样时序和数值

- 采样核在每组开始读取一次 `low_hz`，本组四通道使用同一阈值和时间槽；顺序 R→G→B→Clear，偏移为 0、slot、2×slot、3×slot μs，四通道并不同时测量。
- `slot_us = max(250, ceil(4000000 / low_hz) + 40)`；`period_us = 4 × slot_us`；`sample_rate_hz = floor(1000000 / period_us)`。最短组间隔 1000 μs，最长 4160 μs；实际计划频率为 `1000000 / period_us`，STATUS 使用向下取整的整数 Hz，不把整数 Hz 倒算回组间隔。
- 四个低频周期预算用于滤镜切换响应、等待边沿、丢弃周期和测量，另加 40 μs 软件余量（包含最后 10 μs 整理窗口）。这是采集时间预算，不保证真实硬件在所有低照度/PWM 条件下均无超时。默认 15 kHz 的时间槽为 307 μs，计划间隔 1228 μs，STATUS 为 814 Hz；4 kHz 为 1040 μs / 4160 μs / 240 Hz；20 kHz 及以上为 250 μs / 1000 μs / 1000 Hz。
- STATUS 的阈值与时间槽/采样率取自同一次阈值读取，表示当前请求阈值对应的计划上限。SET_LOW/SET_CONFIG 后，采样核在下一组应用；过渡时相邻组的时间差由前一组周期决定，已经排队的采样不会被追溯更改。
- TCS3200 S0/S1 固定为高（100% 比例），OE 为低。模块照明 LED 不供电；板载 RGB LED 驱动也已移除。
- 切换滤镜后等待 1 μs，PIO 等待低→高，再丢弃一个完整周期，然后测量一个新的上升沿到上升沿周期。每个计数循环 3 个 PIO 周期；默认 125 MHz 下计时粒度约 24 ns，默认输入同步保留。
- PIO 计数值 n 转换为 `cycles=3*n-1`，`hz=floor(clk_sys/cycles)`，`value=floor((hz+5)/10)`；不按 8 位缩放。超过 65535 时饱和到 65535 并设置 clip_mask。
- 每通道在计划起始后 `slot_us - 10` μs 停止 PIO，留出整理结果的时间。未得到完整周期、无边沿或频率低于 low_hz，均将值置零并设置 low_mask。不等待传感器拖延到下一个通道。
- 低频阈值范围为 4～600 kHz；降低阈值会自动延长四通道时间槽，计划组频率不低于 240 Hz。LOW 仍可能来自无光、断线、PWM 或真实切换响应，不能只靠延长时间槽区分原因。
- 如果采样核发现开始/整理时间超出预算，设置 LATE；整组迟到时跳过已过期的计划组，序号跳变，不集中补采。
- 采集队列容量 64 组，USB 发送环容量 4096 字节。队列或发送缓冲满时丢弃整组，不阻塞采样，不输出半帧；已经开始发送的帧会分段继续发送。测试判断独立于 USB 数据是否成功入发送缓冲。

## 测试状态和判定

state：0=WAIT，1=ARM，2=DELAY，3=PRESS，4=HOLD，5=RELEASE。

颜色距离 `d=max(abs(R-Rref), abs(G-Gref), abs(B-Bref))`：

1. WAIT：`d <= enter_tolerance` 连续达到 arm_ms 后进入 ARM。
2. ARM：`d >= exit_threshold` 连续达到 depart_ms 后进入 DELAY。迟滞区间不触发。
3. DELAY：从确认偏离的样本时间戳起计时；到 delay_ms 后等待 HID 就绪并提交按下。偏离已确认后动作锁存，颜色恢复不会取消，模式切换或断连会取消。
4. PRESS：等待按下报告传输完成；成功后进入 HOLD，开始计算 hold_ms。
5. HOLD：到期进入 RELEASE，等待 HID 就绪并发送全零释放报告。
6. RELEASE：报告完成后返回 WAIT；再次匹配 ARM 色才可产生下一次动作。

连续确认使用设备样本时间；相邻采样间隔不等于配置阈值对应的 `period_us`、LATE 或 RGB 的 clip_mask 会重置确认计时。Clear 的 LOW/clip 不影响 RGB 判断。确认时长仍以毫秒配置，但只能在样本到达时确认；例如 240 Hz 下确认 1 ms 仍需要至少两个相隔 4160 μs 的合格样本，不会把未采样的时段当作额外样本。

**LOW 置零后正常参与颜色比较。**因此黑屏可以触发颜色偏离，但无光、传感器断线和部分低频信号也可能表现为零，单靠 OUT 无法区分。参考色与容差应在相同 low_hz 下采集和设置。ARM 状态中的无效（LATE/clip）样本暂停偏离确认，不会直接触发；已经进入 DELAY 的动作不受后续采样影响。

CDC DTR 下降或 USB 挂起时自动进入测量模式，取消未提交的按下，已提交的动作安排释放；USB 挂起期间不能传输，恢复后继续释放，需要显式重新选择测试模式。实际 USB 拔出时清除 HID 状态，重新连接从测量模式开始，RAM 配置仍保留。HID 传输失败会退回测量模式并尝试发送释放，不重发按下。

delay_ms 定义的是设备端提交时刻；USB 忙、1 ms HID 轮询、系统调度都会增加最终应用接收延迟，不能承诺精确的操作系统响应时间。

## 上位机推荐流程

1. 枚举 CDC，打开二进制串口并置 DTR，启动持续接收；GET_STATUS 验证版本和采样率。
2. 测量模式观察 R/G/B/C、LOW/clip/LATE、序号及计数；必要时 SET_LOW，再等待旧数据排空。
3. 在实际屏幕亮度、距离和遮光条件下采集 ARM 参考与波动范围，填写完整 SET_CONFIG，检查 ACK=OK，可 GET_CONFIG 回读核对。
4. SET_MODE=1；继续接收采样与状态，观察 ARM→DELAY→HOLD→WAIT。
5. 停止测试时 SET_MODE=0，等待释放完成（GET_STATUS state=WAIT）后再关闭 CDC 或 BOOTSEL。

不建议普通串口终端打开本设备：显示的是二进制帧。保持 CDC 读取以防主机接收背压，不需要通过 HID 读取任何日志。

## 联调校验向量

以下均为完整帧十六进制，CRC 的字节顺序同样为小端。`make test` 使用 Python 标准库独立验证这些示例的长度、CRC 和字段布局。

```text
GET_STATUS，seq=1：
54 43 02 01 01 00 00 00 b5 59

SET_MODE=测量，seq=2：
54 43 02 02 02 00 01 00 00 de 69

对应 ACK OK，seq=2：
54 43 02 80 02 00 02 00 02 00 2b 15

SET_LOW=15000 Hz，seq=3：
54 43 02 06 03 00 04 00 98 3a 00 00 d7 ea

SAMPLE：seq=0，timestamp=100000，四通道为零，low_mask=0x0F，其余零：
54 43 02 40 00 00 16 00 00 00 00 00 a0 86 01 00 00 00 00 00 00 00 00 00 0f 00 00 00 00 00 68 f3
```

## 资料依据

- [TCS3200 原厂数据手册](https://look.ams-osram.com/m/664723bdb31f55db/original/TCS3200-DS000107.pdf)：滤镜选择、100% 比例、切换响应和周期测量。
- [Pico SDK 硬件 API](https://www.raspberrypi.com/documentation/pico-sdk/hardware.html)：PIO、时钟和双核相关接口。
- [TinyUSB CDC](https://docs.tinyusb.org/en/latest/reference/class/cdc.html) 与 [HID 实现](https://github.com/hathach/tinyusb/tree/master/src/class/hid)：bulk 数据通道、键鼠报告与完成回调。
