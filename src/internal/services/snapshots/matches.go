package snapshots

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"maps"
	"os"
)

// MatchesTree reports whether tarball holds exactly what a snapshot of sshDir
// taken now would: the same archivable files with the same contents. A command
// that failed and left this true changed nothing a snapshot can restore, so
// its snapshot is only taking a retention slot.
func MatchesTree(tarball, sshDir string) bool {
	f, err := os.Open(tarball)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	then, err := fileMembers(f)
	if err != nil {
		return false
	}
	var buf bytes.Buffer
	if err := writeTarGz(&buf, sshDir); err != nil {
		return false
	}
	now, err := fileMembers(&buf)
	if err != nil {
		return false
	}
	return maps.Equal(then, now)
}

// fileMembers is the regular files of a snapshot archive, by name.
func fileMembers(r io.Reader) (map[string]string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		out[hdr.Name] = string(b)
	}
}
