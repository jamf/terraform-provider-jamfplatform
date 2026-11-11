// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

// AlignToPrior orders a server-returned list to follow the order of a prior
// list (the plan during Create/Update, prior state during Read), for a
// collection the server re-sorts but whose entries may repeat.
//
// Such a collection cannot be a Set, because a Set cannot hold two equal
// elements, and a plain List would fail the post-apply consistency check
// whenever the configuration's order differs from the server's sort. Aligning
// on read keeps the List in the order the practitioner wrote, as long as the
// server holds the same entries.
//
// Entries are matched by key. Each key has a first-in, first-out queue of
// prior indexes, so the n-th server entry with a key pairs with the n-th prior
// element with that key. That is correct only when the server keeps insertion
// order among entries sharing a key, so choose the key to be the server's sort
// key and nothing finer: two entries that tie on the sort key must come back in
// the order they were sent.
//
// order lists server indexes in output order and priorIdx gives, for each
// output position, the prior index that entry was paired with, or -1. Matched
// entries come first, in prior order; unmatched server entries follow in server
// order, so an entry the server added surfaces at the end of the list as drift.
// A prior element with no counterpart on the server is dropped. A prior element
// whose key is unknown (priorKey reports false) takes the first unmatched
// server entry, in server order, at its own position; see AlignToPriorFunc to
// constrain that choice.
func AlignToPrior[S, P any](server []S, prior []P, serverKey func(S) string, priorKey func(P) (string, bool)) (order []int, priorIdx []int) {
	return AlignToPriorFunc(server, prior, serverKey, priorKey, nil)
}

// AlignToPriorFunc is AlignToPrior with a predicate for prior elements whose
// key is unknown, which is the normal case on Create when an id is interpolated
// from a resource created in the same apply. Such an element takes the first
// unmatched server entry, in server order, for which fits reports true; a nil
// fits accepts any. Pass a fits that compares the attributes the plan already
// knows, so that two new entries differing in, say, a parameter are not handed
// each other's values.
func AlignToPriorFunc[S, P any](server []S, prior []P, serverKey func(S) string, priorKey func(P) (string, bool), fits func(S, P) bool) (order []int, priorIdx []int) {
	queues := make(map[string][]int, len(prior))
	keyless := make([]bool, len(prior))
	for j, p := range prior {
		k, ok := priorKey(p)
		if !ok {
			keyless[j] = true
			continue
		}
		queues[k] = append(queues[k], j)
	}

	slot := make([]int, len(prior))
	for j := range slot {
		slot[j] = -1
	}
	matched := make([]bool, len(server))
	for s, e := range server {
		k := serverKey(e)
		q := queues[k]
		if len(q) == 0 {
			continue
		}
		slot[q[0]] = s
		queues[k] = q[1:]
		matched[s] = true
	}

	for j, p := range prior {
		if !keyless[j] {
			continue
		}
		for s, e := range server {
			if matched[s] || (fits != nil && !fits(e, p)) {
				continue
			}
			slot[j] = s
			matched[s] = true
			break
		}
	}

	order = make([]int, 0, len(server))
	priorIdx = make([]int, 0, len(server))
	for j, s := range slot {
		if s < 0 {
			continue
		}
		order = append(order, s)
		priorIdx = append(priorIdx, j)
	}
	for s := range server {
		if matched[s] {
			continue
		}
		order = append(order, s)
		priorIdx = append(priorIdx, -1)
	}
	return order, priorIdx
}
