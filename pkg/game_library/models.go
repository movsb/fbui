package game_library

type Kind int32

const (
	KindPlatform Kind = iota + 1
	KindGame
	KindRelease
	KindDeveloper
	KindPublisher
	KindGenre
	KindSeries
)

type Language int32

const (
	LanguageEnglish Language = iota + 1
	LanguageChinese
	LanguageJapanese
)

type AssetType int32

const (
	AssetTypeROM AssetType = iota + 1
	AssetTypeCover
	AssetTypeManual
	AssetTypeScreenshot
	AssetTypeOther
	AssetTypeVideo
	AssetTypeLogo
	AssetTypeSystem
)

type Format int32

const (
	FormatRegular Format = iota + 1
	FormatZIP
)

type Name struct {
	ID       int32
	Kind     Kind
	KindID   int32
	Language Language
	Name     string
	Source   string
}

func (Name) TableName() string { return `names` }

type Platform struct {
	ID    int32
	Names []Name
}

func (Platform) TableName() string { return `platforms` }

type Series struct {
	ID    int32
	Names []Name
}

func (Series) TableName() string { return `series` }

type Game struct {
	ID         int32
	PlatformID int32
	SeriesID   int32
	Names      []Name
}

func (Game) TableName() string { return `games` }

type Release struct {
	ID     int32
	GameID int32
	Names  []Name
}

func (Release) TableName() string { return `releases` }

type Blob struct {
	ID     int32
	Size   int64
	SHA256 string
}

type Entry struct {
	ID      int32
	AssetID int32
	Name    string
	Blob    *Blob
}

type Asset struct {
	ID      int32
	Type    AssetType
	Name    string
	Format  Format
	Size    int64
	Blob    *Blob
	Entries []*Entry
}
