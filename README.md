# vidsim - find similar/duplicate videos in your collection

`vidsim` is a tool that scans a set of video files and identifies videos that are "similar." Since frame-by-frame video comparison is prohibitively slow and doesn't scale well for large collections, this tool takes a pragmatic approach, in that it extracts a single frame from erach video and compares those frames. While this is not as reliable, it works well on typical personal video collections (i.e. not requiring months to run).

## Installation

```sh
go install github.com/abelikoff/vidsim@latest
```

## Operation

The `vidsim` command supports multiple actions described below.

### Find similar videos

In its basic form, `vidsim` scans specified directories and compares all video files, producting a JSON report for all files considered "similar:"

```sh
vidsim process <dir1> <dir2> ...
```

Since frame extraction and comparison are relatively slow and expensive, `vidsim` supports caching of the artifacts it computes, using cached values in future re-runs, which massively speeds up the operation. In order to invoke caching, one specifies a directory to be used for cached data with `-d` option:

```sh
vidsim -d .my.cache.dir process <dir1> <dir2> ...
```

### Handle false positives

Since the comparison logic is imprecise, the will inevitably false positive matches: videos identified as similar, which are not. Running the tool repeatedly and revisiting those false positives again and again is annoying and distracting. To address this, `vidsim` allows marking pairs of videos as false positive matches, so that when it runs next time, this pair of videos will not be reported as a match. Naturally, this is only supported with caching on.

To mark a set of video files as pairwise false positives, use the `unmatch` command:

```sh
vidsim -d .my.cache.dir unmatch <video_file1> <video_file2> ...
```

NOTE: files must be specified using the same path notation as they were handled by the `process` command. For example, if `<dir1>` was specified (among other directories) then the file underneath that directory should be refered to as `<dir1>/subdir1/subdir2/myfile.avi` - i.e. not as a full path or via `./` notation.

## Future work

-   Further optimize Badger usage.
-   Add a `peek` command to examine the cached data.
-   Add a `compact` command to compact the cached data.
-   Add `-debug` command.
-   Display cache statistics during processing.
