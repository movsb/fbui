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
	AssetTypeTitle
)

type Format int32

const (
	FormatRegular Format = iota + 1
	FormatZIP
)

type ReleaseType int32

const (
	ReleaseTypeUnspecified ReleaseType = iota
	ReleaseTypeOriginal
	ReleaseTypeTranslation
	ReleaseTypeModified
	ReleaseTypeOther
)

var ReleaseTypesInDisplayOrder = [...]ReleaseType{
	ReleaseTypeOriginal,
	ReleaseTypeTranslation,
	ReleaseTypeModified,
	ReleaseTypeOther,
	ReleaseTypeUnspecified,
}

func (t ReleaseType) DisplayOrder() int {
	for order, candidate := range ReleaseTypesInDisplayOrder {
		if t == candidate {
			return order
		}
	}
	return len(ReleaseTypesInDisplayOrder) - 1
}

type Name struct {
	ID         int32
	Kind       Kind
	KindID     int32
	Language   Language
	Name       string
	Source     string
	Deprecated bool
}

func (Name) TableName() string { return `names` }

type Platform struct {
	ID          int32
	Description string
	Names       []Name
}

func (Platform) TableName() string { return `platforms` }

type Series struct {
	ID          int32
	Description string
	Names       []Name
}

func (Series) TableName() string { return `series` }

type Game struct {
	ID            int32
	PlatformID    int32
	SeriesID      int32
	Description   string
	Names         []Name
	PlatformNames []Name
}

func (Game) TableName() string { return `games` }

type Release struct {
	ID          int32
	GameID      int32
	Description string
	ReleaseType ReleaseType
	Names       []Name
}

func (Release) TableName() string { return `releases` }

type Blob struct {
	ID     int32
	Size   int64
	SHA256 string
}

func (Blob) TableName() string { return `blobs` }

type Entry struct {
	ID      int32
	AssetID int32
	Name    string
	Size    int64
	BlobID  int32
	Blob    *Blob
}

func (Entry) TableName() string { return `entries` }

type ROMSet struct {
	ID        int32
	Emulator  string
	Version   string
	ShortName string
	AssetID   int32
	CloneOf   string
}

func (ROMSet) TableName() string { return `rom_sets` }

type Asset struct {
	ID          int32
	Kind        Kind
	KindID      int32
	Type        AssetType
	Name        string
	Description string
	Format      Format
	Size        int64
	BlobID      int32
	Blob        *Blob
	Entries     []*Entry
	ROMSets     []*ROMSet
}

func (Asset) TableName() string { return `assets` }
