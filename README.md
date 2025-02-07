# vidsim - find similar/duplicate videos in your collection

`vidsim` is a tool that scans a set of video files and identifies videos that are "similar." Since frame-by-frame video comparison is prohibitively slow and doesn't scale well for large collections, this tool takes a pragmatic approach, in that it extracts a single frame from erach video and compares those frames. While this is not as reliable, it works well on typical personal video collections (i.e. not requiring months to run).

## Installation

```sh
go install github.com/abelikoff/vidsim@latest
```

> [!NOTE] > `vidsim` uses `ffmpeg` tool for frame generation, so make sure the latter is installed as well.

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

## Controlling frame generation

By default `vidsim` calls `ffmpeg` to extract a single image frame from each video file. This behavior can be cusomized by specifying another external program or script via `-G` option. Such program is passed 3 command line arguments:

- A video file to extract the frame from.
- Name of the output frame file.
- A time offset (in format _MM:SS_) at which to extract.

Upon normal completion terminate the program should exit with code 0.

## Controlling the matching logic

### Using external program for comparison

By default `vidsim` uses the [images4](https://github.com/vitali-fedulov/images4) library. One can instead use any external program of choice by passing it to `vidsim` using `--external_comparison_tool` (or `-C`) option. The program is expected to adhere to the following protocol:

- Take two image files as command line parameters.
- Upon normal completion terminate with exit code 0.
- The result of the program execution is a comparison score which the program should output to `stdout`.
- The score is a floating point number between `0` and `1`, where `0` means that the two images don't match at all and `1` means that they match fully.

## Controlling matching logic

### Similarity threshold

Both internal logic and external comparison programs are expected to return a _comparison score_ for a pair of images which is a floating point value between `0` (images don't match at all) and `1` (images math fully). Similarity threshold is the lowest score value at which two images are considered a match. By default it is `0.7` but it can be customized via `--similarity_threshold` option.

## Match grouping logic

When `vidsim` computes pairwise matches of files, it can group matching videos based on different heuristics. This is controlled by `--clustering_mode` (or `-m`) parameter. Currently, `vidsim` offers 3 different heuristics:

- **Strict matching** (`-m strict`) - files are grouped as long as they all match in a pairwise fashion (this is the default behavior).

- **Weak matching** (`-m union`) - assuming match property is transitive, files are groups as long as there is a transitive match between any two files (in other words, as long as `A ^ B, B ^ C, C ^ D` and so on, where `^` denotes the match property)

- **No clustering** (`-m none`) - No grouping happens (each matching pair is reported separately).

### Handling false positives

Since the comparison logic is imprecise, the will inevitably false positive matches: videos identified as similar, which are not. Running the tool repeatedly and revisiting those false positives again and again is annoying and distracting. To address this, `vidsim` allows marking pairs of videos as false positive matches, so that when it runs next time, this pair of videos will not be reported as a match. Naturally, this is only supported with caching on.

To mark a set of video files as pairwise false positives, use the `unmatch` command:

```sh
vidsim -d .my.cache.dir unmatch <video_file1> <video_file2> ...
```

## State management

### Multiple runs with different image comparison parameters

One can do multiple runs using different parameters or tools governing image comparison. To save and use comparison scores for such runs side by side, one can use a "prefix" which is stored along with comparison scores via `--score_prefix` (or `-p`) option, for example:

```bash
# First run with default comparison:
vidsim -d .my.cache.dir process -p run1 <dir1> <dir2> ...

# Second run with more strict comparison logic:
vidsim -d .my.cache.dir process -p run2 --chr_tolerance 5.22 <dir1> <dir2> ...
```

Comparison scores for both runs will be stored in the state without intermixing with each other.

### Compacting the state

State can be compacted, removing data for files that no longer exist:

```sh
vidsim -d .my.cache.dir compact
```

### Dumping the state

You can dump the accumulated state (frame information, comparison scores):

```sh
vidsim -d .my.cache.dir dump
```

### Considerations about filenames

By default `vidsim` saves filenames using relative (to the top directories specified) paths. This has two implications:

-   When operating on files (e.g. marking false positives), paths have to be specified precisely using that convention.
-   When compacting the state, the command should be run _in the same directory where `process` command was run_, otherwise `vidsim` will not find the files listed in the state and will think those files have been deleted (it does have a sanity check agains mass deletion however).

Alternatively, one can specify a `--abs_paths` option to make `vidsim` store absolute paths. This takes more space but it avoids the problem above.
