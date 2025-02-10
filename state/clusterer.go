package state

import (
	"github.com/sirupsen/logrus"
)

type ClusteringMethod int

const (
	// No clustering is done. Each pair is a separate group.
	None ClusteringMethod = iota
	// Only combine elements into a group if all elements match pairwise.
	Strict
	// Union of matched frames. As long as A ^ B and B ^ C, then A, B, C are combined in the same group.
	Loose
)

type Clusterer interface {
	AddMatch(frameID1 int, frameID2 int) // Add a pair of matched frames
	Groups() [][]int                     // Return the list of created groups
	DebugDump()                          // Dump the state
}

// Create a clusterer based on the selected clustering method

func NewClusterer(method ClusteringMethod, logger *logrus.Logger) Clusterer {
	switch method {
	case None:
		return &TrivialClusterer{
			matches: make(map[int]map[int]struct{}),
			logger:  logger,
		}
	case Strict:
		return &StrictClusterer{
			allMatches:  make(map[int]map[int]struct{}),
			groups:      make(map[int][]int),
			nextGroupID: 1,
			logger:      logger,
		}
	case Loose:
		return &UnionClusterer{
			frame2group: make(map[int]int),
			groups:      make(map[int][]int),
			nextGroupID: 1,
			logger:      logger,
		}
	}

	return nil
}

// =====================================================================================

// Trivial clusterer - each match is a separate cluster

type TrivialClusterer struct {
	matches map[int]map[int]struct{} // A match between two frameIDs is encoded by matches[X][Y] being present.
	logger  *logrus.Logger
}

// Add a pair of matched frames

func (clr *TrivialClusterer) AddMatch(frameID1 int, frameID2 int) {
	if frameID1 == frameID2 {
		return
	}

	// Ensure ordering to avoid duplicates

	if frameID1 > frameID2 {
		frameID1, frameID2 = frameID2, frameID1
	}

	// Add the pair to the matches map

	if clr.matches[frameID1] == nil {
		clr.matches[frameID1] = make(map[int]struct{})
	}

	clr.matches[frameID1][frameID2] = struct{}{}
}

// Return the list of created groups

func (clr *TrivialClusterer) Groups() [][]int {
	groups := make([][]int, 0, len(clr.matches))

	for frameID1, frameIDs := range clr.matches {
		for frameID2 := range frameIDs {
			groups = append(groups, []int{frameID1, frameID2})
		}
	}

	return groups
}

// Dump the state

func (clr *TrivialClusterer) DebugDump() {
	clr.logger.Debug("=== Trivial Clusterer ===============================")

	for frameID1, frameIDs := range clr.matches {
		for frameID2 := range frameIDs {
			clr.logger.Debugf("[%d, %d]\n", frameID1, frameID2)
		}
	}

	clr.logger.Debugf("=====================================================")
}

// =====================================================================================

// Strict clusterer - only combine elements into a group if all elements match pairwise

type StrictClusterer struct {
	allMatches  map[int]map[int]struct{} // All matched pairs (encoded by matches[X][Y] being present).
	groups      map[int][]int            // group ID -> list of frame IDs
	nextGroupID int                      // next group ID to assign
	logger      *logrus.Logger
}

// Add a pair of matched frames

func (clr *StrictClusterer) AddMatch(frameID1 int, frameID2 int) {
	clr.logger.Debugf("processing match for %d and %d\n", frameID1, frameID2)

	if frameID1 == frameID2 {
		return
	}

	// Ensure ordering to avoid duplicates

	if frameID1 > frameID2 {
		frameID1, frameID2 = frameID2, frameID1
	}

	// Add the pair to the list of all matches

	if clr.allMatches[frameID1] == nil {
		clr.allMatches[frameID1] = make(map[int]struct{})
	}

	clr.allMatches[frameID1][frameID2] = struct{}{}

	// Incorporate the pair into each group that fully matches both frames

	foundMatches := false

	for groupID, group := range clr.groups {
		clr.logger.Debugf("checking if both %d and %d match group %d  (%v)\n", frameID1, frameID2, groupID, group)

		if clr.frameMatchesGroup(frameID1, groupID) && clr.frameMatchesGroup(frameID2, groupID) {
			clr.groups[groupID] = removeDuplicates(append(group, frameID1, frameID2))
			clr.logger.Debugf("GROUP MATCH: added both %d and %d to group %d  ==>  (%v)\n",
				frameID1, frameID2, groupID, clr.groups[groupID])
			foundMatches = true
		}
	}

	if foundMatches {
		return
	}

	// no existing group matches - create a new group
	clr.groups[clr.nextGroupID] = []int{frameID1, frameID2}
	clr.logger.Debugf("created new group for %d and %d ==> group %d (%v)\n",
		frameID1, frameID2, clr.nextGroupID, clr.groups[clr.nextGroupID])
	clr.nextGroupID++
}

// Return the list of created groups

func (clr *StrictClusterer) Groups() [][]int {
	groups := make([][]int, 0, len(clr.groups))

	for _, group := range clr.groups {
		groups = append(groups, group)
	}

	return groups
}

// Dump the state

func (clr *StrictClusterer) DebugDump() {
	clr.logger.Debug("=== Strict Clusterer ===============================")
	clr.logger.Debug("--- all matches ---")

	for frameID1, frameIDs := range clr.allMatches {
		for frameID2 := range frameIDs {
			clr.logger.Debugf("[%d, %d]\n", frameID1, frameID2)
		}
	}

	clr.logger.Debug("--- groups ---")

	for groupID, group := range clr.groups {
		clr.logger.Debugf("group %d:  %v\n", groupID, group)
	}

	clr.logger.Debug("=====================================================")
}

// Check if a pair of frame IDs match

func (clr *StrictClusterer) match(frameID1 int, frameID2 int) bool {
	if frameID1 == frameID2 {
		return true
	}

	if frameID1 > frameID2 {
		frameID1, frameID2 = frameID2, frameID1
	}

	if _, found := clr.allMatches[frameID1][frameID2]; found {
		return true
	}

	return false
}

// Check if a frame matches a group

func (clr *StrictClusterer) frameMatchesGroup(frameID int, groupID int) bool {
	if group, found := clr.groups[groupID]; found {
		for _, groupFrameID := range group {
			if !clr.match(frameID, groupFrameID) {
				return false
			}
		}
	}

	return true
}

func removeDuplicates(array []int) []int {
	seen := make(map[int]bool)
	result := []int{}

	for _, num := range array {
		if !seen[num] {
			seen[num] = true
			result = append(result, num)
		}
	}

	return result
}

// =====================================================================================

// Union clusterer - union of matched frames. As long as A ^ B and B ^ C, then A, B, C are combined in the same group.

type UnionClusterer struct {
	frame2group map[int]int   // frame ID -> group ID
	groups      map[int][]int // group ID -> list of frame IDs
	nextGroupID int           // next group ID to assign
	logger      *logrus.Logger
}

// Add a pair of matched frames

func (clr *UnionClusterer) AddMatch(frameID1 int, frameID2 int) {
	if frameID1 == frameID2 {
		return
	}

	// Ensure ordering to avoid duplicates

	if frameID1 > frameID2 {
		frameID1, frameID2 = frameID2, frameID1
	}

	resultingBucket := -1
	group1 := -1
	group2 := -1

	if group, found := clr.frame2group[frameID1]; found {
		group1 = group
	}

	if group, found := clr.frame2group[frameID2]; found {
		group2 = group
	}

	// both frames are already grouped - union those two groups

	if group1 >= 0 && group2 >= 0 {
		if group1 == group2 {
			return
		}

		// merge group1 into group2

		for frameID, group := range clr.frame2group {
			if group == group1 {
				clr.frame2group[frameID] = group2
			}
		}

		clr.groups[group2] = append(clr.groups[group2], clr.groups[group1]...)
		delete(clr.groups, group1)
		return
	}

	if group1 >= 0 {
		resultingBucket = group1
	} else if group2 >= 0 {
		resultingBucket = group2
	} else { // neither frame is in a group - create a new group
		resultingBucket = clr.nextGroupID
		clr.nextGroupID++
	}

	clr.frame2group[frameID1] = resultingBucket
	clr.frame2group[frameID2] = resultingBucket

	if clr.groups[resultingBucket] == nil {
		clr.groups[resultingBucket] = []int{frameID1, frameID2}
	} else {
		clr.groups[resultingBucket] = append(clr.groups[resultingBucket], frameID1, frameID2)
	}
}

// Return the list of created groups

func (clr *UnionClusterer) Groups() [][]int {
	groups := make([][]int, 0, len(clr.groups))

	for _, group := range clr.groups {
		groups = append(groups, group)
	}

	return groups
}

// Dump the state

func (clr *UnionClusterer) DebugDump() {
	clr.logger.Debug("=== Union Clusterer ===============================")

	for groupID, group := range clr.groups {
		clr.logger.Debugf("group %d:  %v\n", groupID, group)
	}

	clr.logger.Debugf("=====================================================")
}
