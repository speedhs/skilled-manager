package core

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExportZip writes a ZIP archive of the specified skills (or all skills if skillNames is empty) to writer.
func (m *Manager) ExportZip(skillNames []string, writer io.Writer) error {
	var targetSkills []string

	if len(skillNames) == 0 {
		skills, err := ListSkills(m.CanonicalDir)
		if err != nil {
			return fmt.Errorf("failed to list skills for export: %w", err)
		}
		for _, s := range skills {
			targetSkills = append(targetSkills, s.Name)
		}
	} else {
		targetSkills = skillNames
	}

	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	for _, name := range targetSkills {
		skillDir := filepath.Join(m.CanonicalDir, name)
		info, err := os.Stat(skillDir)
		if err != nil {
			return fmt.Errorf("skill %q not found at %s: %w", name, skillDir, err)
		}
		if !info.IsDir() {
			continue
		}

		err = filepath.Walk(skillDir, func(path string, fileInfo os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(m.CanonicalDir, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)

			header, err := zip.FileInfoHeader(fileInfo)
			if err != nil {
				return err
			}
			header.Name = relPath

			if fileInfo.IsDir() {
				header.Name += "/"
				_, err = zipWriter.CreateHeader(header)
				return err
			}

			header.Method = zip.Deflate
			w, err := zipWriter.CreateHeader(header)
			if err != nil {
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(w, file)
			return err
		})

		if err != nil {
			return fmt.Errorf("failed to add skill %q to zip: %w", name, err)
		}
	}

	return nil
}
