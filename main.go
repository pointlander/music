// Copyright 2020 The Music Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/dhowden/tag"
)

var (
	// Out is the output directory
	Out = flag.String("out", "", "output directory")
	// Convert converts the file to a different type
	Convert = flag.String("convert", "", "type to convert to")
)

func main() {
	flag.Parse()

	var walk func(input, output string)
	walk = func(input, output string) {
		in, err := os.Open(input)
		if err != nil {
			panic(err)
		}
		defer in.Close()
		names, err := in.Readdirnames(0)
		if err != nil {
			panic(err)
		}
		sort.Strings(names)
		flacs := make([]string, 0, 8)
		for _, name := range names {
			if name == "." || name == ".." {
				continue
			}
			path := filepath.Join(input, name)
			file, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			info, err := file.Stat()
			if err != nil {
				file.Close()
				panic(err)
			}
			if info.IsDir() {
				dir := filepath.Join(output, strings.ToLower(name))
				fmt.Printf("mkdir %s\n", dir)
				err := os.Mkdir(dir, info.Mode())
				if err != nil {
					file.Close()
					panic(err)
				}
				walk(path, dir)
			} else if strings.HasSuffix(name, ".flac") {
				flacs = append(flacs, name)
			} else {
				new := filepath.Join(output, name)
				fmt.Printf("cp %s %s\n", path, new)
				cp, err := os.Create(new)
				if err != nil {
					file.Close()
					panic(err)
				}
				_, err = io.Copy(cp, file)
				if err != nil {
					file.Close()
					cp.Close()
					panic(err)
				}
				cp.Close()
			}
			file.Close()
		}

		max := 0
		for _, name := range flacs {
			if len(name) > max {
				max = len(name)
			}
		}
		prefix, hasNumbers := "", false
		if len(flacs) > 0 {
		search:
			for i := 0; i < max; i++ {
				var common rune = -1
				for _, name := range flacs {
					if common == -1 {
						if i < len(name) {
							if unicode.IsNumber([]rune(name)[i]) {
								common = -2
							} else {
								common = []rune(name)[i]
							}
						}
					} else if common == -2 {
						if !unicode.IsNumber([]rune(name)[i]) {
							break search
						}
					} else if []rune(name)[i] != common {
						break search
					}
				}
				if common == -2 {
					hasNumbers = true
					break search
				}
				prefix += fmt.Sprintf("%v", common)
			}
		}

		maxTrack, maxDisc := 0, 0
		for _, name := range flacs {
			path := filepath.Join(input, name)
			file, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			metadata, err := tag.ReadFrom(file)
			if err != nil {
				panic(err)
			}
			file.Close()

			track, tracks := metadata.Track()
			disc, discs := metadata.Disc()
			if track > maxTrack {
				maxTrack = track
			}
			if tracks-1 > maxTrack {
				maxTrack = tracks - 1
			}
			if disc > maxDisc {
				maxDisc = disc
			}
			if discs-1 > maxDisc {
				maxDisc = discs - 1
			}
		}
		trackPadding, discPadding :=
			fmt.Sprintf("%d", int(math.Log10(float64(maxTrack)))+1),
			fmt.Sprintf("%d", int(math.Log10(float64(maxDisc)))+1)

		done := make(chan bool, 8)
		process := func(name string) {
			path := filepath.Join(input, name)
			file, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			defer file.Close()

			if !hasNumbers {
				metadata, err := tag.ReadFrom(file)
				if err != nil {
					panic(err)
				}
				_, err = file.Seek(0, 0)
				if err != nil {
					panic(err)
				}
				track, _ := metadata.Track()
				disc, _ := metadata.Disc()
				if maxDisc == 0 {
					name = fmt.Sprintf("%0"+trackPadding+"d_%s", track, name)
				} else {
					name = fmt.Sprintf("%0"+discPadding+"d_%0"+trackPadding+"d_%s", disc, track, name)
				}
			}

			if *Convert != "" {
				new := filepath.Join(output, strings.TrimSuffix(name, ".flac")+"."+*Convert)
				fmt.Println("ffmpeg", "-i", path /*"-ab", "320k",*/, "-map_metadata", "0", "-id3v2_version", "3", new)
				command := exec.Command("ffmpeg", "-i", path /*"-ab", "320k",*/, "-map_metadata", "0", "-id3v2_version", "3", new)
				err := command.Run()
				if err != nil {
					panic(err)
				}
			} else {
				new := filepath.Join(output, name)
				fmt.Printf("cp %s %s\n", path, new)
				cp, err := os.Create(new)
				if err != nil {
					panic(err)
				}
				defer cp.Close()
				_, err = io.Copy(cp, file)
				if err != nil {
					panic(err)
				}
			}

			done <- true
		}

		for _, name := range flacs {
			go process(name)
		}
		for range flacs {
			<-done
		}
	}
	walk(".", *Out)
}
