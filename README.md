# trimui

## 从 GM 远程启动 ROM

GM 二进制优先从 SD 卡的 `Blobs/<sha256 前两位>/<完整 sha256>` 读取，可将 GM blobs 目录的内容原样复制到 `sdcard/Blobs/`，保留子目录和 gzip 文件格式。文件校验通过后直接使用；缺失或损坏时通过 HTTP 下载、校验并保存到同一目录。数据库和组装后的资源仍位于 `.fbui/gm/`。

FBUI 在现有 `0.0.0.0:8888` HTTP 服务上提供 Store ROM 启动接口：

```bash
curl -X POST http://DEVICE_IP:8888/api/store/assets/123:open
```

成功会立即返回 `202 Accepted` 和 `{"status":"accepted","asset_id":123}`。这只表示任务已经受理；资源下载、校验、ZIP 组装和模拟器启动会继续在设备端执行。接口只接受属于 Release 的 ROM asset，其他资源不会打开；同时只能执行一个远程打开任务。

接口允许 GM 管理页面跨域调用且没有鉴权，只应在可信局域网中启用。
