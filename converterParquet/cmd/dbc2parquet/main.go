// Command dbc2parquet converts one or more .dbc parts (same logical base) to a single UTF-8 column Parquet file without SQL Server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"converterParquet/internal/convert"
)

func main() {
	out := flag.String("o", "", "output .parquet path")
	timeout := flag.Int("timeout", 600, "per-conversion timeout in seconds")
	flag.Parse()
	args := flag.Args()
	if *out == "" || len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: dbc2parquet -o out.parquet [-timeout sec] in1.dbc [in2.dbc ...]\n")
		os.Exit(2)
	}
	wopt := convert.WriterOptions{
		ParallelWriters: 4,
		RowGroupSize:    128 * 1024 * 1024,
		PageSize:        8 * 1024,
	}
	ctx := context.Background()
	dur := time.Duration(*timeout) * time.Second
	log.Printf("dbc2parquet: merging %d part(s) -> %s (timeout=%v)", len(args), *out, dur)
	n, err := convert.MergeDBCsToParquet(ctx, *out, args, dur, wopt)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d data rows)", *out, n)
}
