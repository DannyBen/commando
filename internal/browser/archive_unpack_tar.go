package browser

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func unpackTAR(filename, directory, name string, compressed bool) (Entry, error) {
	source, err := os.Open(filename)
	if err != nil {
		return Entry{}, err
	}
	defer source.Close()
	plan := newArchiveExtractPlan()
	members := map[string]*tar.Header{}
	// Scan before creating output, including the gzip checksum. Keep metadata,
	// never payloads; extraction makes a second streaming pass on the same file.
	if err := scanTAR(source, compressed, func(header *tar.Header, _ io.Reader) error {
		if err := plan.add(header.Name, header.FileInfo().Mode()); err != nil {
			return err
		}
		members[header.Name] = header
		return nil
	}); err != nil {
		return Entry{}, err
	}
	return unpackDirectory(directory, name, func(root *os.Root) error {
		if err := prepareArchiveExtraction(root, plan.tops); err != nil {
			return err
		}
		err := scanTAR(source, compressed, func(header *tar.Header, data io.Reader) error {
			before := members[header.Name]
			if before == nil || before.Size != header.Size || before.Typeflag != header.Typeflag {
				return ErrArchiveChanged
			}
			delete(members, header.Name)
			local := filepath.FromSlash(header.Name)
			if header.Typeflag == tar.TypeDir {
				return root.MkdirAll(local, 0700)
			}
			if err := root.MkdirAll(filepath.Dir(local), 0700); err != nil {
				return err
			}
			return extractArchiveFile(root, local, data, before.ModTime)
		})
		if err == nil && len(members) != 0 {
			err = ErrArchiveChanged
		}
		return err
	})
}

func scanTAR(source *os.File, compressed bool, visit func(*tar.Header, io.Reader) error) error {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var stream io.Reader = source
	if compressed {
		reader, err := gzip.NewReader(source)
		if err != nil {
			return err
		}
		defer reader.Close()
		stream = reader
	}
	reader := tar.NewReader(stream)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			// TAR EOF precedes its padding and the gzip trailer. Consume these
			// too, so truncation/checksum failures cannot look like success.
			_, err = io.Copy(io.Discard, stream)
			return err
		}
		if err != nil {
			return err
		}
		for strings.HasPrefix(header.Name, "./") {
			header.Name = strings.TrimPrefix(header.Name, "./")
		}
		header.Name = strings.TrimSuffix(header.Name, "/")
		if header.Typeflag == tar.TypeDir && (header.Name == "." || header.Name == "") {
			continue // Common root-directory records from `tar ... .`.
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
			return errors.ErrUnsupported
		}
		if err := visit(header, reader); err != nil {
			return err
		}
	}
}
