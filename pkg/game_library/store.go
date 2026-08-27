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
func (s *Store) ensureBlob(ctx context.Context, expected *pb.Blob) (string, error) {
	if expected == nil || expected.GetId() <= 0 || !sha256Pattern.MatchString(expected.GetSha256()) || expected.GetSize() < 0 {
		return "", errors.New("invalid blob metadata")
	}
	// Store downloads are deliberately serialized in v1. The mutex also prevents
	// two assets sharing a blob from racing over the same .part file.
	s.mu.Lock()
	defer s.mu.Unlock()

	final := s.blobPath(expected.GetSha256())

	// 如果本地有此二进制，校验并直接使用。
	if validBlob(final, expected) {
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

	hash := sha256.New()
	closeAndRemove := func(downloadErr error) (string, error) {
		file.Close()
		os.Remove(temporary)
		return "", downloadErr
	}

	reader, err := s.source.Open(ctx, expected.GetId())
	if err != nil {
		return closeAndRemove(err)
	}
	written, copyErr := io.Copy(io.MultiWriter(file, hash), reader)
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

func validBlob(path string, expected *pb.Blob) bool {
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
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return fmt.Sprintf("%x", hash.Sum(nil)) == expected.GetSha256()
}

// 从本地读取或者远程下载后并重新组装成所需求的资源文件。
// 返回资源文件的路径。
func (s *Store) Materialize(ctx context.Context, asset *pb.Asset) (string, error) {
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
		blob, err := s.ensureBlob(ctx, asset.GetBlob())
		if err != nil {
			return "", err
		}
		final := filepath.Join(dir, name)
		if validBlob(final, asset.GetBlob()) {
			return final, nil
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
		temporary := final + ".part"
		os.Remove(temporary)
		file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		archive := zip.NewWriter(file)
		seen := map[string]bool{}
		fail := func(buildErr error) (string, error) {
			archive.Close()
			file.Close()
			os.Remove(temporary)
			return "", buildErr
		}
		for _, entry := range asset.GetEntries() {
			entryName, err := safeEntryName(entry.GetName())
			if err != nil || seen[entryName] {
				if err == nil {
					err = fmt.Errorf("duplicate zip entry: %s", entryName)
				}
				return fail(err)
			}
			seen[entryName] = true
			blob, err := s.ensureBlob(ctx, entry.GetBlob())
			if err != nil {
				return fail(err)
			}
			input, err := os.Open(blob)
			if err != nil {
				return fail(err)
			}
			output, err := archive.Create(entryName)
			if err == nil {
				_, err = io.Copy(output, input)
			}
			closeErr := input.Close()
			if err != nil {
				return fail(err)
			}
			if closeErr != nil {
				return fail(closeErr)
			}
		}
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
		return final, nil
	default:
		return "", fmt.Errorf("unsupported asset format: %s", asset.GetFormat())
	}
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
