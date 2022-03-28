package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/disintegration/imaging"
	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	"github.com/jdeng/goheif"
	"github.com/kolesa-team/go-webp/encoder"
	"github.com/kolesa-team/go-webp/webp"
)

var pre *string
var dpr *int
var qual *int
var widths []int
var wg sync.WaitGroup

var stdoutMutex sync.Mutex

type filePath struct {
	Dpr  uint
	Path string
}

type imageFilePath struct {
	FileType  string
	Width     uint
	FilePaths []filePath
}

func main() {
	flag.Usage = usage
	pre = flag.String("pre", "/", "The HTML prefix to the image path")
	dpr = flag.Int("dpr", 3, "The DPR to start generating images with")
	qual = flag.Int("qual", 25, "The quality of JPEG to encode (worst 1 <-> 100 best)")
	widthFlag := flag.String("widths", "288", "The pixel widths separated by commas that you want to generate")
	flag.Parse()

	widths = getWidths(widthFlag)
	files := flag.Args()

	if len(files) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if *qual < 1 || *qual > 100 {
		fmt.Fprintf(os.Stderr, "invalid quality setting: %d. Must be value 1-100\n", *qual)
		flag.Usage()
		os.Exit(1)
	}

	if *dpr < 1 {
		fmt.Fprintf(os.Stderr, "invalid dpr setting: %d. Must be value 1 or greater\n", *dpr)
		flag.Usage()
		os.Exit(1)
	}

	// fmt.Println("Prefix:", *pre)
	// fmt.Println("DPR:", *dpr)
	// fmt.Println("Qual:", *qual)
	// for _, width := range widths {
	// 	fmt.Println("Width:", width)
	// }

	for _, file := range files {
		wg.Add(1)
		go generate(file)
	}

	wg.Wait()

}

func getWidths(widths *string) []int {
	widthStringSlice := strings.Split(*widths, ",")
	widthIntSlice := make([]int, len(widthStringSlice))
	for i, widthString := range widthStringSlice {
		parsedInt, err := strconv.Atoi(widthString)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid, non-integer width provided: %s, ignoring\n", widthString)
			continue
		}
		widthIntSlice[i] = parsedInt
	}
	return widthIntSlice
}

func usage() {
	fmt.Printf("Usage: %s [OPTIONS] inputFile [inputFile ...]\n", os.Args[0])
	flag.PrintDefaults()
}

func generate(file string) {
	fmt.Println("Generating output for file:", file)
	imageFilePaths := make([]imageFilePath, 0)
	defer wg.Done()
	orient, err := getOrientation(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not get orientation data for file %s\n", file)
	}
	img, err := getImage(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not get image data for file %s\n", file)
		return
	}
	img, err = transformAccordingly(img, orient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not transform image data for file %s\n", file)
		return
	}
	for _, width := range widths {
		imgs := resizeAccordingly(img, width)
		ifps, err := writeImages(file, imgs, width)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not write output images for file %s\n", file)
			return
		}
		imageFilePaths = append(imageFilePaths, ifps...)
	}
	err = printHTML(imageFilePaths, file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not write HTML picture element for file %s\n", file)
		return
	}
}

func printHTML(ifps []imageFilePath, file string) error {
	lastIndex := len(ifps) - 1
	lastWidthIndex := lastIndex
	lastWidth := ifps[lastWidthIndex].Width
	for i := lastWidthIndex; i >= 0; i-- {
		if ifps[i].Width == lastWidth {
			lastWidthIndex = i
		} else {
			break
		}
	}
	templateString := `<picture>
{{- range $i, $ifp := .Ifps -}}
{{- if (ne $i $.LastIndex) }}
	<source {{- if lt $i $.LastWidthIndex }} media="(min-width: )"{{ end }} type="image/{{$ifp.FileType}}" srcset="{{ range $j, $e := $ifp.FilePaths }}{{ if (ne $j 0) }}, {{ end }}{{ $e.Path }}{{ if (ne $e.Dpr 1) }} {{ $e.Dpr }}x{{ end }}{{ end }}">
{{- else }}
	<img {{range $j, $e := $ifp.FilePaths }}{{ if (eq $j 0) }}src="{{ $e.Path }}{{ if (gt (len $ifp.FilePaths) 1) }}" srcset="{{ end }}{{ else }}{{ if (ne $j 1) }}, {{ end }}{{ $e.Path }} {{ $e.Dpr}}x{{ end }}{{ end }}" alt="" width="{{ $ifp.Width }}">
{{- end -}}
{{- end }}
</picture>
`

	tmpl, err := template.New("picture").Parse(templateString)
	if err != nil {
		return err
	}
	stdoutMutex.Lock()
	fmt.Println("HTML for file:", file)
	err = tmpl.Execute(os.Stdout, struct {
		Ifps           []imageFilePath
		LastIndex      uint
		LastWidthIndex uint
	}{
		Ifps:           ifps,
		LastIndex:      uint(lastIndex),
		LastWidthIndex: uint(lastWidthIndex),
	})
	stdoutMutex.Unlock()
	if err != nil {
		return err
	}
	return nil
}

func writeImages(file string, imgs []image.Image, width int) ([]imageFilePath, error) {
	imageFilePaths := make([]imageFilePath, 0)
	extension := filepath.Ext(file)
	dirName := strings.TrimSuffix(file, extension)
	baseDir := filepath.Base(dirName)
	err := os.MkdirAll(dirName, 0755)
	if err != nil {
		return nil, err
	}
	fileType := strings.ToLower(strings.Trim(extension, "."))
	switch fileType {
	case "heic", "jpg":
		jpegFilePaths := make([]filePath, 0)
		webpFilePaths := make([]filePath, 0)
		for i := 1; i <= *dpr; i++ {
			fileNameNoExt := filepath.Join(dirName, fmt.Sprintf("%dw%dd", width, i))
			fileName := fmt.Sprintf("%s.jpg", fileNameNoExt)
			err = writeJpg(fileName, imgs[i-1])
			if err != nil {
				return nil, err
			}
			jpegFilePaths = append(jpegFilePaths, filePath{Dpr: uint(i), Path: filepath.Join(*pre, baseDir, fmt.Sprintf("%dw%dd.jpg", width, i))})
			fileName = fmt.Sprintf("%s.webp", fileNameNoExt)
			err = writeWebp(fileName, imgs[i-1])
			if err != nil {
				return nil, err
			}
			webpFilePaths = append(webpFilePaths, filePath{Dpr: uint(i), Path: filepath.Join(*pre, baseDir, fmt.Sprintf("%dw%dd.webp", width, i))})
		}
		imageFilePaths = append(imageFilePaths, imageFilePath{FileType: "webp", Width: uint(width), FilePaths: webpFilePaths})
		imageFilePaths = append(imageFilePaths, imageFilePath{FileType: "jpeg", Width: uint(width), FilePaths: jpegFilePaths})
	case "png":
		pngFilePaths := make([]filePath, 0)
		for i := 1; i <= *dpr; i++ {
			fileNameNoExt := filepath.Join(dirName, fmt.Sprintf("%dw%dd", width, i))
			fileName := fmt.Sprintf("%s.png", fileNameNoExt)
			err = writePng(fileName, imgs[i-1])
			if err != nil {
				return nil, err
			}
			pngFilePaths = append(pngFilePaths, filePath{Dpr: uint(i), Path: filepath.Join(*pre, baseDir, fmt.Sprintf("%dw%dd.png", width, i))})
		}
		imageFilePaths = append(imageFilePaths, imageFilePath{FileType: "png", Width: uint(width), FilePaths: pngFilePaths})
	default:
		return nil, fmt.Errorf("i don't know how to handle %s files", fileType)
	}
	return imageFilePaths, nil
}

func writePng(file string, img image.Image) error {
	outFile, err := os.Create(file)
	if err != nil {
		return err
	}
	defer outFile.Close()
	return png.Encode(outFile, img)
}

func writeWebp(file string, img image.Image) error {
	outFile, err := os.Create(file)
	if err != nil {
		return err
	}
	defer outFile.Close()
	options, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, float32(*qual))
	if err != nil {
		return err
	}
	return webp.Encode(outFile, img, options)
}

func writeJpg(file string, img image.Image) error {
	outFile, err := os.Create(file)
	if err != nil {
		return err
	}
	defer outFile.Close()
	return jpeg.Encode(outFile, img, &jpeg.Options{Quality: *qual})
}

func resizeAccordingly(img image.Image, width int) []image.Image {
	images := make([]image.Image, 0)
	for i := 1; i <= *dpr; i++ {
		images = append(images, imaging.Resize(img, width*i, 0, imaging.Lanczos))
	}
	return images
}

func transformAccordingly(img image.Image, orient uint16) (image.Image, error) {
	switch orient {
	case 1:
		if imgNrgba, ok := img.(*image.NRGBA); ok {
			return imgNrgba, nil
		} else {
			return nil, errors.New("could not generate nrgba image")
		}
	case 2:
		// flip horizontally
		return imaging.FlipH(img), nil
	case 3:
		// rotate 180
		return imaging.Rotate180(img), nil
	case 4:
		// rotate 180
		// flip horizontally
		newImg := imaging.Rotate180(img)
		return imaging.FlipH(newImg), nil
	case 5:
		// rotate 270
		// flip horizontally
		newImg := imaging.Rotate270(img)
		return imaging.FlipH(newImg), nil
	case 6:
		// rotate 270
		return imaging.Rotate270(img), nil
	case 7:
		// rotate 90
		// flip horizontally
		newImg := imaging.Rotate90(img)
		return imaging.FlipH(newImg), nil
	case 8:
		// rotate 90
		return imaging.Rotate90(img), nil
	default:
		if imgNrgba, ok := img.(*image.NRGBA); ok {
			return imgNrgba, fmt.Errorf("cannot work with orientation %d for image", orient)
		} else {
			return nil, fmt.Errorf("cannot work with orientation %d for image", orient)
		}
	}
}

func getImage(file string) (image.Image, error) {
	extension := filepath.Ext(file)
	fileType := strings.ToLower(strings.Trim(extension, "."))
	switch fileType {
	case "jpg":
		return getImageFromJpeg(file)
	case "png":
		return getImageFromPng(file)
	case "heic":
		return getImageFromHeic(file)
	default:
		return nil, fmt.Errorf("i don't know how to handle %s files", fileType)
	}
}

func getImageFromPng(file string) (image.Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func getImageFromJpeg(file string) (image.Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return jpeg.Decode(f)
}

func getImageFromHeic(file string) (image.Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return goheif.Decode(f)
}

func getOrientation(file string) (uint16, error) {
	exifByteSlice, err := exif.SearchFileAndExtractExif(file)
	if err != nil {
		return 1, err
	}
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return 1, err
	}
	ti := exif.NewTagIndex()
	_, index, err := exif.Collect(im, ti, exifByteSlice)
	if err != nil {
		return 1, err
	}

	orient := uint16(1)

	err = index.RootIfd.EnumerateTagsRecursively(func(i *exif.Ifd, ite *exif.IfdTagEntry) error {
		if ite.TagName() == "Orientation" {
			val, err := ite.Value()
			if err != nil {
				return err
			}

			switch val := val.(type) {
			case []uint16:
				if len(val) != 1 {
					return fmt.Errorf("orientation captures multiple values %s", file)
				}
				orient = val[0]
			case uint16:
				orient = val
			default:
				return fmt.Errorf("orientation value is not of right type %s", file)
			}
		}
		return nil
	})
	if err != nil {
		return 1, err
	}
	return orient, nil
}
