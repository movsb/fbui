# 视频播放器

反汇编官方的视频播放器、`libtplayer.so`、`libxplayer.so`和`libuapi.so`后创建的一个视频播放器。

## 工具链

[trimui/toolchain_sdk_smartpro: SDK and toolchain for TRIMUI Smart Pro TG5040](https://github.com/trimui/toolchain_sdk_smartpro)

正常来说，仅gcc之类的来说，是通用的。我的系统是brick pro，用的是smart pro的（因为官方没提供brick pro的）。

直接放到编译机器上即可（linux，amd64）。

## 依赖

因为参数中写了允许未定义的依赖，所以实际上只需要把`libtplayer.so`拷贝到编译机即可。位于: `/usr/lib`。

## 编译

```bash
$ export TOOLCHAIN="$HOME/aarch64-linux-gnu-7.5.0-linaro"
$ export PATH="$TOOLCHAIN/bin:$PATH"
$
$ aarch64-linux-gnu-gcc player.c -Os -ffunction-sections -fdata-sections -fno-unwind-tables -fno-asynchronous-unwind-tables -Wl,--gc-sections -Wl,--allow-shlib-undefined -L. -ltplayer -o player
$ aarch64-linux-gnu-strip --strip-all player
```

## 使用

```bash
usage: ./player video.mp4 x y width height

example:
  ./player video.mp4 100 80 824 600

fullscreen:
  ./player video.mp4 0 0 1024 768
```
