package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Value struct {
	Station string
	Sum     float32
	Count   uint32
	Max     float32
	Min     float32
}

type LineSplit struct {
	Station    []byte
	Temprature []byte
}

func splitLine(line []byte) (LineSplit, error) {
	station, temprature, found := bytes.Cut(line, []byte(";"))
	if !found {
		return LineSplit{}, fmt.Errorf("invalid line: %s", line)
	} else {
		return LineSplit{Station: station, Temprature: temprature}, nil
	}
}

func SplitWorker(wg *sync.WaitGroup, lineChannel <-chan []byte, decodeChannel chan<- LineSplit) {
	defer wg.Done()
	for line := range lineChannel {
		lineSplit, err := splitLine(line)
		if err != nil {
			fmt.Println(err)
			continue
		}

		decodeChannel <- lineSplit
	}
}

func FileWorker(wg *sync.WaitGroup, start, stop int64, lineChannel chan<- []byte) {
	defer wg.Done()
	f, err := os.Open("measurements.txt")
	if err != nil {
		panic(err)
	}

	defer f.Close()

	r := bufio.NewReader(f)
	if start != 0 {
		_, err := f.Seek(start-int64(len("\n")), io.SeekStart)
		if err != nil {
			panic(err)
		}
		r.Reset(f)
		newline := make([]byte, len("\n"))
		_, err = r.Read(newline)
		if err != nil {
			panic(err)
		}
		if string(newline) != "\n" {
			line, err := r.ReadBytes('\n')
			if err != nil {
				panic(err)
			}
			start += int64(len(line))
		}
	}

	for {
		if start >= stop {
			break
		}

		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			start += int64(len(line))
			lineChannel <- bytes.TrimRight(line, "\n")
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			panic(err)
		}
	}
}

func decodeLine(lineSplit LineSplit, result map[string]*Value) error {
	station := string(lineSplit.Station)
	elm := result[station]
	if elm == nil {
		elm = new(Value)
		elm.Min = -100
		elm.Max = 100
		elm.Station = station
		result[station] = elm
	}

	value, err := strconv.ParseFloat(
		string(lineSplit.Temprature),
		32,
	)
	if err != nil {
		return err
	}
	value32 := float32(value)
	elm.Sum += value32
	elm.Count += 1
	if value32 > elm.Max {
		elm.Max = value32
	}

	if value32 < elm.Min {
		elm.Min = value32
	}
	return nil
}

func DecodeWorker(wg *sync.WaitGroup, decodeChannel <-chan LineSplit, result map[string]*Value) {
	defer wg.Done()
	for line := range decodeChannel {
		err := decodeLine(line, result)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func main() {
	start := time.Now()
	fStat, err := os.Stat("measurements.txt")
	if err != nil {
		panic(err)
	}

	fileWg := new(sync.WaitGroup)
	splitWg := new(sync.WaitGroup)
	decodeWg := new(sync.WaitGroup)
	lineChannel := make(chan []byte, 10000)
	decodeChannel := make(chan LineSplit, 10000)
	mergedResult := make(map[string]*Value)
	var workerResult []map[string]*Value

	var prevOffset int64
	fileWorkers := 1
	for i := 0; i < fileWorkers; i++ {
		fileWg.Add(1)
		nextOffset := prevOffset + int64(
			float64(1)/float64(fileWorkers)*float64(fStat.Size()),
		)
		if i+1 == fileWorkers {
			nextOffset = fStat.Size()
		}

		go FileWorker(fileWg, prevOffset, nextOffset, lineChannel)
		prevOffset = nextOffset
	}

	splitWorkers := 1
	for i := 0; i < splitWorkers; i++ {
		splitWg.Add(1)
		go SplitWorker(splitWg, lineChannel, decodeChannel)
	}

	decodeWorkers := 1
	for i := 0; i < decodeWorkers; i++ {
		decodeWg.Add(1)
		decodeResult := make(map[string]*Value)
		workerResult = append(workerResult, decodeResult)
		go DecodeWorker(decodeWg, decodeChannel, decodeResult)
	}

	fileWg.Wait()
	close(lineChannel)
	splitWg.Wait()
	close(decodeChannel)
	decodeWg.Wait()
	processTime := time.Now()

	for _, decodeResult := range workerResult {
		for k, v := range decodeResult {
			if resultV := mergedResult[k]; resultV == nil {
				mergedResult[k] = v
			} else {
				resultV.Sum += v.Sum
				resultV.Count += v.Count
				if v.Max > resultV.Max {
					resultV.Max = v.Max
				}

				if v.Min < resultV.Min {
					resultV.Min = v.Min
				}
			}
		}
	}

	var resultList []*Value
	for _, value := range mergedResult {
		resultList = append(resultList, value)
	}

	mergeTime := time.Now()
	sort.Slice(resultList, func(i, j int) bool {
		return resultList[i].Station < resultList[j].Station
	})

	sortTime := time.Now()
	output, err := os.Create("output.csv")
	if err != nil {
		panic(err)
	}
	defer output.Close()

	for _, value := range resultList {
		_, err := fmt.Fprintf(output, "%s;%f;%f;%f\n", value.Station, value.Sum/float32(value.Count), value.Min, value.Max)
		if err != nil {
			panic(err)
		}
	}

	outputTime := time.Now()
	fmt.Printf("process time: %v\n", processTime.Sub(start))
	fmt.Printf("merge time: %v\n", mergeTime.Sub(processTime))
	fmt.Printf("sort time: %v\n", sortTime.Sub(mergeTime))
	fmt.Printf("output time: %v\n", outputTime.Sub(sortTime))
	fmt.Printf("total time: %v\n", outputTime.Sub(start))
}
