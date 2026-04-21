package picker

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	pickerLogPrefix          = "region-picker:"
	lastRegionFileName       = ".last-region"
	filePerm                 = 0o644
	pickerDirectoryPerm      = 0o755
	rootRegionID             = ""
	pathSeparator            = " > "
	regionPbfSuffix          = "-latest.osm.pbf"
	mapBuildMessage          = "Building graph for %s (this may take several minutes)...\n"
	downloadMessage          = "\nDownloading %s...\n"
	chooseRegionPrompt       = "\nSelect region: "
	chooseBrowsePrompt       = "\nSelect: "
	chooseCombinedPrompt     = "Choose: "
	downloadNewRegionOption  = "n"
	browseParentOption       = "b"
	browseSubregionsOption   = "1"
	downloadWholeRegionInput = "2"
)

const (
	geofabrikLogPrefix = "region-picker: geofabrik:"
	indexFilePerm      = 0o644
	geofabrikStatusOK  = http.StatusOK
	rootParentID       = ""
)

// Feature represents a single Geofabrik region entry.
type Feature struct {
	ID     string
	Name   string
	Parent string
	PBFUrl string
}

// Index is the parsed Geofabrik catalog.
type Index struct {
	ByID     map[string]Feature
	Children map[string][]Feature
}

type geofabrikIndexJSON struct {
	Features []struct {
		Properties struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Parent string `json:"parent"`
			URLs   struct {
				PBF string `json:"pbf"`
			} `json:"urls"`
		} `json:"properties"`
	} `json:"features"`
}

// FetchIndex loads the Geofabrik index from a fresh cache entry or downloads it.
func FetchIndex(cacheFile string, maxAge time.Duration) (*Index, error) {
	if info, err := os.Stat(cacheFile); err == nil {
		if time.Since(info.ModTime()) < maxAge {
			if idx, err := loadIndexFromFile(cacheFile); err == nil {
				log.Printf("%s using cached index %s", geofabrikLogPrefix, cacheFile)
				return idx, nil
			}
		}
	}

	indexURL := GeofabrikIndexURL()
	log.Printf("%s downloading index from %s", geofabrikLogPrefix, indexURL)
	client := &http.Client{Timeout: indexRequestTimeout}
	resp, err := client.Get(indexURL) //nolint:noctx // one-shot CLI download
	if err != nil {
		return nil, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != geofabrikStatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	if err := os.WriteFile(cacheFile, body, indexFilePerm); err != nil {
		log.Printf("%s could not update cache %s: %v", geofabrikLogPrefix, cacheFile, err)
	}

	return parseIndex(body)
}

func loadIndexFromFile(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseIndex(data)
}

func parseIndex(data []byte) (*Index, error) {
	var raw geofabrikIndexJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing index JSON: %w", err)
	}

	idx := &Index{
		ByID:     make(map[string]Feature, len(raw.Features)),
		Children: make(map[string][]Feature),
	}

	for _, f := range raw.Features {
		p := f.Properties
		if p.ID == "" {
			continue
		}
		feat := Feature{
			ID:     p.ID,
			Name:   p.Name,
			Parent: p.Parent,
			PBFUrl: p.URLs.PBF,
		}
		idx.ByID[feat.ID] = feat
		parentID := feat.Parent
		if parentID == rootParentID {
			parentID = rootParentID
		}
		idx.Children[parentID] = append(idx.Children[parentID], feat)
	}

	return idx, nil
}
