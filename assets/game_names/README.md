# Game Names

压缩包文件名到中文名/英文名的映射关系，从`/usr/trimui/lib/libgamename.so`中提取。

## 编译

Go不允许cc和go放同一个文件夹（否则需要被编译），所以放src了。

```bash
$ g++ dump.cc
```

## 生成

```bash
$ ./a.out zh | gzip -9 > zh.tsv.gz
```
