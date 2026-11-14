package seq

import "slices"

// Diff lists what changed between two parses of a growing source, by the
// IDs of participants ("p:" and the name) and of events.
type Diff struct {
	Added, Changed, Removed []string
}

// Empty reports whether nothing changed.
func (d Diff) Empty() bool { return len(d.Added)+len(d.Changed)+len(d.Removed) == 0 }

// key identifies an event: the end of a block has its block's ID.
func key(e *Event) string {
	if e.Kind == BlockEnd {
		return e.ID + ".end"
	}
	return e.ID
}

// Compare returns what changed from old to cur. A nil old is an empty
// diagram.
func Compare(old, cur *Diagram) Diff {
	if old == nil {
		old = &Diagram{}
	}
	var d Diff
	was := map[string]*Participant{}
	for _, p := range old.Participants {
		was[p.ID] = p
	}
	now := map[string]bool{}
	for _, p := range cur.Participants {
		now[p.ID] = true
		switch o := was[p.ID]; {
		case o == nil:
			d.Added = append(d.Added, "p:"+p.ID)
		case *o != *p:
			d.Changed = append(d.Changed, "p:"+p.ID)
		}
	}
	for _, p := range old.Participants {
		if !now[p.ID] {
			d.Removed = append(d.Removed, "p:"+p.ID)
		}
	}
	wasEv := map[string]*Event{}
	for _, e := range old.Events {
		wasEv[key(e)] = e
	}
	nowEv := map[string]bool{}
	for _, e := range cur.Events {
		k := key(e)
		nowEv[k] = true
		switch o := wasEv[k]; {
		case o == nil:
			d.Added = append(d.Added, k)
		case !sameEvent(o, e):
			d.Changed = append(d.Changed, k)
		}
	}
	for _, e := range old.Events {
		if !nowEv[key(e)] {
			d.Removed = append(d.Removed, key(e))
		}
	}
	if old.Title != cur.Title || len(old.Boxes) != len(cur.Boxes) {
		d.Changed = append(d.Changed, "diagram")
	}
	return d
}

func sameEvent(a, b *Event) bool {
	return a.Kind == b.Kind && a.From == b.From && a.To == b.To && a.Dotted == b.Dotted && a.Head == b.Head &&
		a.Both == b.Both && a.Number == b.Number && a.ActivateTo == b.ActivateTo && a.DeactivateFrom == b.DeactivateFrom &&
		a.Placement == b.Placement && slices.Equal(a.Actors, b.Actors) && a.Actor == b.Actor &&
		a.Block == b.Block && a.Color == b.Color && a.Text == b.Text
}
