package fonts

import (
	"embed"
	"os"
	"runtime"

	"github.com/movsb/fbiw"
)

//go:embed mono.ttf nerd.ttf
var userRoot embed.FS

var sysRoot = fbiw.Iif(runtime.GOOS == `linux`, os.DirFS(`/usr/trimui/res`), fbiw.CallerDir())

func Init(app *fbiw.App) {
	app.AddFontFile(`system`, false, false, sysRoot, `full.ttf`)
	app.AddFontFile(`nerd`, false, false, userRoot, `nerd.ttf`)
	app.AddFontFile(`monospace`, false, false, userRoot, `mono.ttf`)
}
