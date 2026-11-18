# 协议与展示素材

## 开发文档

[USB 二进制协议 v4](protocol.md) 定义固件和上位机共用的帧格式、命令、采样单位、状态机与联调校验向量。修改协议时，同步更新两端实现与测试；不兼容更改须提升帧版本。

- [固件说明](../firmware/README.md)：接线、构建、刷写和上板验证。
- [上位机说明](../host/README.md)：连接、波形操作、触发测试、颜色校准和开发验证。
- [外壳设计](../3D/README.md) 与 [PCB 设计](../layout/README.md)：硬件源文件和使用说明。

## 展示图片

`images/` 保存软件界面和屏幕采样素材，集中展示见 [项目首页](../README.md#软件与实测)。

- [screenshot.png](images/screenshot.png)：软件界面截图。
- [screen200_sample1633_photo.jpg](images/screen200_sample1633_photo.jpg)：在 200 Hz 刷新率的屏幕上，以 1633 Hz 采样率采集 UFO Test 页面的实拍照片。
- [screen200_sample1633_chart.png](images/screen200_sample1633_chart.png)：上述实拍采样的数值波形截图。
- [screen200_sample1633_fft_chart.png](images/screen200_sample1633_fft_chart.png)：上述实拍采样的 FFT 频谱截图，与数值波形在项目首页并列展示。
