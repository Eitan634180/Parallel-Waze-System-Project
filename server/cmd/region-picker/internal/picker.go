package picker

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nav-system/src/mapdata"
)

var pickerUI = os.Stderr

func Run() {
	mapRoot := flag.String("map-root", mapdata.DefaultMapRoot(), "Directory containing region subdirectories")
	cacheFile := flag.String("cache", DefaultGeofabrikCacheFile(), "Local cache file for the Geofabrik index")
	regionID := flag.String("region", "", "Non-interactive: download and build this Geofabrik region ID")
	prompt := flag.Bool("prompt", false, "Always show the selection menu even if a last region is remembered")
	keepPBF := flag.Bool("keep-pbf", true, "Keep the downloaded .pbf file after building")
	flag.Parse()

	if err := os.MkdirAll(*mapRoot, pickerDirectoryPerm); err != nil {
		log.Fatalf("%s cannot create map root: %v", pickerLogPrefix, err)
	}
	if err := os.MkdirAll(filepath.Dir(*cacheFile), pickerDirectoryPerm); err != nil {
		log.Fatalf("%s cannot create data dir: %v", pickerLogPrefix, err)
	}
	lastRegionPath := filepath.Join(filepath.Dir(*cacheFile), lastRegionFileName)

	if *regionID != "" {
		runNonInteractive(*regionID, *mapRoot, *cacheFile, lastRegionPath, *keepPBF)
		return
	}

	regions, err := ListReady(*mapRoot)
	if err != nil {
		log.Fatalf("%s scanning map root: %v", pickerLogPrefix, err)
	}

	if len(regions) == 0 {
		runDownloadFlow(*mapRoot, *cacheFile, lastRegionPath, *keepPBF)
		return
	}

	if !*prompt && len(regions) > 1 {
		if last := readLastRegion(lastRegionPath); last != "" {
			for _, region := range regions {
				if region.Dir == last {
					fmt.Println(region.Dir)
					log.Printf("%s auto-selected remembered region: %s", pickerLogPrefix, region.ID)
					return
				}
			}
		}
	}

	if len(regions) == 1 && !*prompt {
		fmt.Println(regions[0].Dir)
		log.Printf("%s auto-selected only region: %s", pickerLogPrefix, regions[0].ID)
		writeLastRegion(lastRegionPath, regions[0].Dir)
		return
	}

	chosen, downloadRequested := selectExistingRegion(regions)
	if downloadRequested {
		runDownloadFlow(*mapRoot, *cacheFile, lastRegionPath, *keepPBF)
		return
	}

	writeLastRegion(lastRegionPath, chosen.Dir)
	fmt.Println(chosen.Dir)
}

func runNonInteractive(regionID, mapRoot, cacheFile, lastRegionPath string, keepPBF bool) {
	log.Printf("%s non-interactive mode: region=%s", pickerLogPrefix, regionID)

	dir := filepath.Join(mapRoot, regionID)
	if IsReady(dir) {
		log.Printf("%s region already built: %s", pickerLogPrefix, dir)
		fmt.Println(dir)
		writeLastRegion(lastRegionPath, dir)
		return
	}

	idx, err := FetchIndex(cacheFile, indexMaxAge)
	if err != nil {
		log.Fatalf("%s fetching Geofabrik index: %v", pickerLogPrefix, err)
	}

	feature, ok := idx.ByID[regionID]
	if !ok {
		log.Fatalf("%s unknown region ID %q. Check %s", pickerLogPrefix, regionID, GeofabrikIndexURL())
	}
	if feature.PBFUrl == "" {
		log.Fatalf("%s region %q has no PBF download URL", pickerLogPrefix, regionID)
	}

	dir = downloadAndBuild(feature, mapRoot, keepPBF)
	writeLastRegion(lastRegionPath, dir)
	fmt.Println(dir)
}

func runDownloadFlow(mapRoot, cacheFile, lastRegionPath string, keepPBF bool) {
	log.Printf("%s starting download flow", pickerLogPrefix)

	idx, err := FetchIndex(cacheFile, indexMaxAge)
	if err != nil {
		log.Fatalf("%s fetching Geofabrik index: %v", pickerLogPrefix, err)
	}

	feature := browseIndex(idx)
	dir := downloadAndBuild(feature, mapRoot, keepPBF)
	writeLastRegion(lastRegionPath, dir)
	fmt.Println(dir)
}

func downloadAndBuild(feature Feature, mapRoot string, keepPBF bool) string {
	dir := filepath.Join(mapRoot, feature.ID)
	if err := os.MkdirAll(dir, pickerDirectoryPerm); err != nil {
		log.Fatalf("%s creating region dir %s: %v", pickerLogPrefix, dir, err)
	}

	pbfPath := filepath.Join(dir, feature.ID+regionPbfSuffix)
	if _, err := os.Stat(pbfPath); os.IsNotExist(err) {
		uiPrintf(downloadMessage, feature.Name)
		if err := DownloadPBF(feature.PBFUrl, pbfPath, pickerUI); err != nil {
			log.Fatalf("%s download failed: %v", pickerLogPrefix, err)
		}
		uiPrintln("")
	} else {
		log.Printf("%s using existing PBF: %s", pickerLogPrefix, pbfPath)
	}

	uiPrintf(mapBuildMessage, feature.Name)
	if err := runMapBuilder(pbfPath, dir); err != nil {
		log.Fatalf("%s map build failed: %v", pickerLogPrefix, err)
	}

	if !keepPBF {
		if err := os.Remove(pbfPath); err != nil {
			log.Printf("%s could not remove PBF %s: %v", pickerLogPrefix, pbfPath, err)
		}
	}

	return dir
}

func selectExistingRegion(regions []RegionInfo) (RegionInfo, bool) {
	uiPrintln("\nAvailable regions:")
	for i, region := range regions {
		uiPrintf("  [%d] %s\n", i+1, region.ID)
	}
	uiPrintf("  [%s] Download a new region\n", downloadNewRegionOption)

	for {
		uiPrint(chooseRegionPrompt)
		line := strings.TrimSpace(readLine())

		if strings.EqualFold(line, downloadNewRegionOption) {
			return RegionInfo{}, true
		}

		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(regions) {
			uiPrintf("Please enter a number between 1 and %d, or '%s'.\n", len(regions), downloadNewRegionOption)
			continue
		}
		return regions[n-1], false
	}
}

func browseIndex(idx *Index) Feature {
	current := rootRegionID
	var breadcrumb []string

	for {
		children := idx.Children[current]
		if len(children) == 0 {
			log.Fatalf("%s no regions found in catalog", pickerLogPrefix)
		}

		if len(breadcrumb) == 0 {
			uiPrintln("\nSelect a continent or region:")
		} else {
			uiPrintf("\n%s > Select a region:\n", strings.Join(breadcrumb, pathSeparator))
		}

		for i, feature := range children {
			marker := ""
			if feature.PBFUrl != "" && len(idx.Children[feature.ID]) == 0 {
				marker = " *"
			}
			uiPrintf("  [%d] %s%s\n", i+1, feature.Name, marker)
		}
		if len(breadcrumb) > 0 {
			uiPrintf("  [%s] Back\n", browseParentOption)
		}

		uiPrint(chooseBrowsePrompt)
		line := strings.TrimSpace(readLine())

		if strings.EqualFold(line, browseParentOption) && len(breadcrumb) > 0 {
			breadcrumb = breadcrumb[:len(breadcrumb)-1]
			if len(breadcrumb) == 0 {
				current = rootRegionID
			} else if parent, ok := idx.ByID[current]; ok {
				current = parent.Parent
			}
			continue
		}

		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(children) {
			uiPrintf("Please enter a number between 1 and %d.\n", len(children))
			continue
		}

		chosen := children[n-1]
		hasChildren := len(idx.Children[chosen.ID]) > 0
		hasPBF := chosen.PBFUrl != ""

		if hasChildren && hasPBF {
			uiPrintf("\n%q has both sub-regions and a full download.\n", chosen.Name)
			uiPrintf("  [%s] Browse sub-regions\n", browseSubregionsOption)
			uiPrintf("  [2] Download entire %s\n", chosen.Name)
			uiPrint(chooseCombinedPrompt)
			if strings.TrimSpace(readLine()) == downloadWholeRegionInput {
				return chosen
			}
			breadcrumb = append(breadcrumb, chosen.Name)
			current = chosen.ID
			continue
		}

		if hasChildren {
			breadcrumb = append(breadcrumb, chosen.Name)
			current = chosen.ID
			continue
		}

		if !hasPBF {
			uiPrintf("%q has no PBF download. Please choose a more specific region.\n", chosen.Name)
			continue
		}

		return chosen
	}
}

func readLine() string {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			ch := buf[0]
			if ch == '\n' {
				break
			}
			if ch != '\r' {
				sb.WriteByte(ch)
			}
		}
		if err != nil {
			break
		}
	}
	return sb.String()
}

func writeLastRegion(path, dir string) {
	if err := os.WriteFile(path, []byte(dir), filePerm); err != nil {
		log.Printf("%s could not save last region %s: %v", pickerLogPrefix, path, err)
	}
}

func readLastRegion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func uiPrintln(line string) {
	fmt.Fprintln(pickerUI, line)
}

func uiPrint(line string) {
	fmt.Fprint(pickerUI, line)
}

func uiPrintf(format string, args ...any) {
	fmt.Fprintf(pickerUI, format, args...)
}
