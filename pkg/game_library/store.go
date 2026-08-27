package game_library

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	pb "github.com/movsb/gm/protocols/go/proto"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// 远程二进制存储。
type BlobSource interface {
	// 通过 Blob ID 下载二进制。
	Open(context.Context, int32) (io.ReadCloser, error)
}

type Store struct {
	root   string
	source BlobSource
	mu     sync.Mutex
}

func New(root string, source BlobSource) *Store {
	return &Store{root: root, source: source}
}

func (s *Store) blobPath(sha string) string {
	return filepath.Join(s.root, "blobs", sha[:2], sha)
}

// 确保本地存储有此二进制。
// 如果没有，会从远程下载并存储到本地。
// 下载后会完整哈希校验。
func (s *Store) ensureBlob(ctx context.Context, expected *pb.Blob, local, remote func(p float32)) (string, error) {
	if expected == nil || expected.GetId() <= 0 || !sha256Pattern.MatchString(expected.GetSha256()) || expected.GetSize() < 0 {
		return "", errors.New("invalid blob metadata")
	}
	// Store downloads are deliberately serialized in v1. The mutex also prevents
	// two assets sharing a blob from racing over the same .part file.
	s.mu.Lock()
	defer s.mu.Unlock()

	final := s.blobPath(expected.GetSha256())

	// 如果本地有此二进制，校验并直接使用。
	if validBlob(final, expected, local) {
		return final, nil
	}

	// 需要重新下载。
	os.Remove(final)
	if err := os.MkdirAll(filepath.Dir(final), 0755); err != nil {
		return "", err
	}

	// 下载到临时文件。
	temporary := final + ".part"
	os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}

	var (
		hash           = sha256.New()
		remoteProgress = &_ProgressWriter{
			total:    int(expected.GetSize()),
			progress: remote,
		}
		closeAndRemove = func(downloadErr error) (string, error) {
			file.Close()
			os.Remove(temporary)
			return "", downloadErr
		}
	)

	reader, err := s.source.Open(ctx, expected.GetId())
	if err != nil {
		return closeAndRemove(err)
	}
	written, copyErr := io.Copy(io.MultiWriter(file, hash, remoteProgress), reader)
	closeErr := reader.Close()
	if copyErr != nil {
		return closeAndRemove(copyErr)
	}
	if closeErr != nil {
		return closeAndRemove(closeErr)
	}
	if written != int64(expected.GetSize()) || fmt.Sprintf("%x", hash.Sum(nil)) != expected.GetSha256() {
		return closeAndRemove(errors.New("downloaded blob failed size or sha256 verification"))
	}
	if err := file.Sync(); err != nil {
		return closeAndRemove(err)
	}
	if err := file.Close(); err != nil {
		os.Remove(temporary)
		return "", err
	}
	if err := os.Rename(temporary, final); err != nil {
		os.Remove(temporary)
		return "", err
	}
	return final, nil
}

func validBlob(path string, expected *pb.Blob, progress func(p float32)) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != int64(expected.GetSize()) {
		return false
	}
	hash := sha256.New()
	pw := &_ProgressWriter{
		total:    int(expected.GetSize()),
		progress: progress,
	}
	if _, err := io.Copy(io.MultiWriter(hash, pw), file); err != nil {
		return false
	}
	return fmt.Sprintf("%x", hash.Sum(nil)) == expected.GetSha256()
}

// 从本地读取或者远程下载后并重新组装成所需求的资源文件。
// 返回资源文件的本地固定路径。
func (s *Store) Materialize(ctx context.Context, asset *pb.Asset, progress func(message string, p float32)) (string, error) {
	if asset == nil || asset.GetId() <= 0 {
		return "", errors.New("invalid asset")
	}
	dir := filepath.Join(s.root, "assets", fmt.Sprint(asset.GetId()))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name, err := safeBaseName(asset.GetName())
	if err != nil {
		return "", err
	}
	switch asset.GetFormat() {
	case pb.Format_FORMAT_REGULAR:
		// 有就直接使用。
		final := filepath.Join(dir, name)
		if validBlob(final, asset.GetBlob(), func(p float32) {
			progress(`校验文件`, p)
		}) {
			return final, nil
		}
		// 如果没有才考虑重新物化/下载。
		blob, err := s.ensureBlob(ctx, asset.GetBlob(),
			func(p float32) {
				progress(`校验文件`, p)
			},
			func(p float32) {
				progress(`下载文件`, p)
			},
		)
		if err != nil {
			return "", err
		}
		os.Remove(final)
		if err := os.Link(blob, final); err != nil {
			if err := copyFile(blob, final); err != nil {
				return "", err
			}
		}
		return final, nil
	case pb.Format_FORMAT_ZIP:
		if !strings.HasSuffix(strings.ToLower(name), ".zip") {
			name += ".zip"
		}
		final := filepath.Join(dir, name)
		shaPath := filepath.Join(dir, "sha.txt")
		if validZIPCache(final, shaPath, func(p float32) {
			progress(`校验文件`, p)
		}) {
			return final, nil
		}

		// 第一轮确保所有 Entry 对应的二进制都已经下载并校验到本地。
		type localEntry struct {
			name string
			path string
		}
		entries := make([]localEntry, 0, len(asset.GetEntries()))
		seen := map[string]bool{}
		var totalSize int64
		for _, entry := range asset.GetEntries() {
			entryName, err := safeEntryName(entry.GetName())
			if err != nil {
				return "", err
			}
			if seen[entryName] {
				return "", fmt.Errorf("duplicate zip entry: %s", entryName)
			}
			seen[entryName] = true
			totalSize += int64(entry.GetBlob().GetSize())
		}
		var completedSize int64
		for _, entry := range asset.GetEntries() {
			blobSize := int64(entry.GetBlob().GetSize())
			report := func(message string, entryProgress float32) {
				if totalSize <= 0 {
					progress(message, 100)
					return
				}
				current := float64(completedSize) + float64(blobSize)*float64(entryProgress)/100
				progress(message, float32(current/float64(totalSize)*100))
			}
			blob, err := s.ensureBlob(ctx, entry.GetBlob(),
				func(p float32) { report(`校验文件`, p) },
				func(p float32) { report(`下载文件`, p) },
			)
			if err != nil {
				return "", err
			}
			entryName, _ := safeEntryName(entry.GetName())
			entries = append(entries, localEntry{name: entryName, path: blob})
			completedSize += blobSize
		}

		// 第二轮只使用本地二进制组装 ZIP，并单独报告组装进度。
		temporary := final + ".part"
		os.Remove(temporary)
		file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		archive := zip.NewWriter(file)
		fail := func(buildErr error) (string, error) {
			archive.Close()
			file.Close()
			os.Remove(temporary)
			return "", buildErr
		}
		assembleProgress := &_ProgressWriter{
			total: int(totalSize),
			progress: func(p float32) {
				progress(`重新组装`, p)
			},
		}
		for _, entry := range entries {
			input, err := os.Open(entry.path)
			if err != nil {
				return fail(err)
			}
			output, err := archive.Create(entry.name)
			if err == nil {
				_, err = io.Copy(io.MultiWriter(output, assembleProgress), input)
			}
			closeErr := input.Close()
			if err != nil {
				return fail(err)
			}
			if closeErr != nil {
				return fail(closeErr)
			}
		}
		progress(`重新组装`, 100)
		if err := archive.Close(); err != nil {
			_ = file.Close()
			_ = os.Remove(temporary)
			return "", err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			_ = os.Remove(temporary)
			return "", err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(temporary)
			return "", err
		}
		if err := os.Rename(temporary, final); err != nil {
			_ = os.Remove(temporary)
			return "", err
		}
		sha, err := fileSHA256(final, nil)
		if err != nil {
			return "", err
		}
		if err := writeSHAFile(shaPath, sha); err != nil {
			return "", err
		}
		return final, nil
	default:
		return "", fmt.Errorf("unsupported asset format: %s", asset.GetFormat())
	}
}

func validZIPCache(zipPath, shaPath string, progress func(float32)) bool {
	expected, err := os.ReadFile(shaPath)
	if err != nil {
		return false
	}
	expectedSHA := strings.TrimSpace(string(expected))
	if !sha256Pattern.MatchString(expectedSHA) {
		return false
	}
	actualSHA, err := fileSHA256(zipPath, progress)
	return err == nil && actualSHA == expectedSHA
}

func fileSHA256(path string, progress func(float32)) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if progress == nil {
		_, err = io.Copy(hash, file)
	} else {
		info, statErr := file.Stat()
		if statErr != nil {
			return "", statErr
		}
		_, err = io.Copy(io.MultiWriter(hash, &_ProgressWriter{total: int(info.Size()), progress: progress}), file)
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func writeSHAFile(path, sha string) error {
	return os.WriteFile(path, []byte(sha+"\n"), 0644)
}

func safeBaseName(name string) (string, error) {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", errors.New("invalid asset name")
	}
	return name, nil
}

func safeEntryName(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if name == "" || strings.HasPrefix(name, "/") || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("unsafe zip entry name")
	}
	return clean, nil
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(destination)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return nil
}

type _ProgressWriter struct {
	count    int
	total    int
	progress func(p float32)
}

func (w *_ProgressWriter) Write(p []byte) (int, error) {
	w.count += len(p)
	if w.progress != nil {
		w.progress(float32(w.count) / float32(w.total) * 100)
	}
	return len(p), nil
}
