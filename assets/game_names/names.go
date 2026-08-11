package game_names

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strings"
	"sync"
)

// 返回翻译过的归档文件名。
// 如果没有此翻译，返回空。
func Translate(archiveName string) string {
	archiveName = strings.ToLower(archiveName)
	prefix, _ := strings.CutSuffix(archiveName, `.zip`)
	return nameMaps()[prefix]
}

//go:embed zh.tsv.gz
var data []byte

var nameMaps = sync.OnceValue(func() map[string]string {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		panic(`读游戏名字数据失败:` + err.Error())
	}
	defer gr.Close()

	nm := map[string]string{}

	scanner := bufio.NewScanner(gr)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")
		// 应该只有最后一行空行才会这样
		if len(parts) != 2 {
			continue
		}
		archive := strings.ToLower(parts[0])
		translated := parts[1]
		if existed, ok := nm[archive]; ok {
			_ = existed
			// log.Printf(`存在同名的游戏名字: %s, %s, %s`, archive, existed, translated)
			// 后面的可能要新一点，不continue，直接覆盖
			// continue
		}
		nm[archive] = translated
	}
	if scanner.Err() != nil {
		panic(`游戏名字数据有误:` + scanner.Err().Error())
	}

	return nm
})
