package regions

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	downloadLogPrefix         = "region-picker: download:"
	mapBuilderBinaryName      = "map-builder"
	tempDownloadPattern       = "download-*.tmp"
	directoryPerm             = 0o755
	downloadIndent            = "\r  "
	downloadProgressFormat    = downloadIndent + "%.0f%% (%s / %s)   "
	downloadUnknownSizeFormat = downloadIndent + "%s downloaded   "
	downloadCompleteFormat    = downloadIndent + "100%% (%s)                    \n"
	downloadExistsFormat      = downloadIndent + "File already exists: %s\n"
	downloadFinishedFormat    = downloadIndent + "%s downloaded\n"
	httpByteUnit              = 1024
	serverSearchMaxDepth      = 6
	goCommandName             = "go"
	goRunCommand              = "./cmd/map-builder"
	mapBuilderArgPBF          = "--pbf"
	mapBuilderArgOut          = "--out"
)

// DownloadPBF downloads the file at url to destPath with an atomic rename.
func DownloadPBF(url, destPath string, w io.Writer) error {
	dir := filepath.Dir(destPath)

	if err := os.MkdirAll(dir, directoryPerm); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	if _, err := os.Stat(destPath); err == nil {
		fmt.Fprintf(w, downloadExistsFormat, destPath)
		return nil
	}

	tmp, err := os.CreateTemp(dir, tempDownloadPattern)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if _, err := os.Stat(tmpName); err == nil {
			os.Remove(tmpName)
		}
	}()

	resp, err := http.Get(url) //nolint:noctx // one-shot CLI download
	if err != nil {
		tmp.Close()
		return fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	total := resp.ContentLength // -1 if unknown
	pw := &progressWriter{w: tmp, progress: w, total: total}

	if _, err := io.Copy(pw, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("downloading: %w", err)
	}
	tmp.Close()

	if total > 0 {
		fmt.Fprintf(w, downloadCompleteFormat, humanBytes(total))
	} else {
		fmt.Fprintf(w, downloadFinishedFormat, humanBytes(pw.written))
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

type progressWriter struct {
	w        io.Writer
	progress io.Writer
	total    int64
	written  int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	pw.written += int64(n)
	if pw.total > 0 {
		pct := float64(pw.written) / float64(pw.total) * 100
		fmt.Fprintf(pw.progress, downloadProgressFormat, pct, humanBytes(pw.written), humanBytes(pw.total))
	} else {
		fmt.Fprintf(pw.progress, downloadUnknownSizeFormat, humanBytes(pw.written))
	}
	return n, err
}

func humanBytes(b int64) string {
	if b < httpByteUnit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(httpByteUnit), 0
	for n := b / httpByteUnit; n >= httpByteUnit; n /= httpByteUnit {
		div *= httpByteUnit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// runMapBuilder preprocesses a downloaded PBF into the binary graph format.
func runMapBuilder(pbfPath, outDir string) error {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), mapBuilderBinaryName)
		if _, serr := os.Stat(candidate); serr == nil {
			return execCmd(candidate, mapBuilderArgPBF, pbfPath, mapBuilderArgOut, outDir)
		}
		candidateExe := candidate + ".exe"
		if _, serr := os.Stat(candidateExe); serr == nil {
			return execCmd(candidateExe, mapBuilderArgPBF, pbfPath, mapBuilderArgOut, outDir)
		}
	}

	log.Printf("%s map-builder binary not found; falling back to 'go run'", downloadLogPrefix)
	serverDir, err := findServerDir()
	if err != nil {
		return fmt.Errorf("cannot locate server source: %w", err)
	}
	return execCmdDir(serverDir, goCommandName, "run", goRunCommand, mapBuilderArgPBF, pbfPath, mapBuilderArgOut, outDir)
}

func execCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func execCmdDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findServerDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	candidates := []string{filepath.Dir(exe)}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd, filepath.Join(cwd, "server"))
	}

	for _, base := range candidates {
		dir := base
		for i := 0; i < serverSearchMaxDepth; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return "", fmt.Errorf("go.mod not found near %s", exe)
}
