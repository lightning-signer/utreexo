package utreexo

import (
	"fmt"
	"sync"
)

// Modify is the main function that deletes then adds elements to the accumulator
func (p *Pollard) Modify(adds []LeafTXO, dels []uint64) error {
	err := p.rem2(dels)
	if err != nil {
		return err
	}
	// fmt.Printf("pol pre add %s", p.toString())
	err = p.add(adds)
	if err != nil {
		return err
	}

	return nil
}

// Stats :
func (p *Pollard) Stats() string {
	s := fmt.Sprintf("pol nl %d tops %d he %d re %d ow %d \n",
		p.numLeaves, len(p.tops), p.hashesEver, p.rememberEver, p.overWire)
	return s
}

// Add a leaf to a pollard.  Not as simple!
func (p *Pollard) add(adds []LeafTXO) error {

	// General algo goes:
	// 1 make a new node & assign data (no neices; at bottom)
	// 2 if this node is on a height where there's already a top,
	// then swap neices with that top, hash the two datas, and build a new
	// node 1 higher pointing to them.
	// goto 2.

	// this does everything 1 at a time, with hashing also mixed in, so that's
	// pretty sub-optimal, but we're not doing multi-thread yet

	for _, a := range adds {

		//		if p.numLeaves < p.Minleaves ||
		//			(add.Duration < p.Lookahead && add.Duration > 0) {
		//			remember = true
		//			p.rememberEver++
		//		}
		if a.Remember {
			p.rememberEver++
		}

		err := p.addOne(a.Hash, a.Remember)
		if err != nil {
			return err
		}
	}
	//	fmt.Printf("added %d, nl %d tops %d\n", len(adds), p.numLeaves, len(p.tops))
	return nil
}

/*
Algo explanation with catchy terms: grab, swap, hash, new, pop
we're iterating through the tops of the pollard.  Tops correspond with 1-bits
in numLeaves.  As soon as we hit a 0 (no top), we're done.

grab: Grab the lowest top.
pop: pop off the lowest top.
swap: swap the neices of the node we grabbed and our new node
hash: calculate the hashes of the old top and new node
new: create a new parent node, with the hash as data, and the old top / prev new node
as neices (not neices though, children)

It's pretty dense: very little code but a bunch going on.

Not that `p.tops = p.tops[:len(p.tops)-1]` would be a memory leak (I guess?)
but that leftTop is still being pointed to anyway do it's OK.

*/

// add a single leaf to a pollard
func (p *Pollard) addOne(add Hash, remember bool) error {
	// basic idea: you're going to start at the LSB and move left;
	// the first 0 you find you're going to turn into a 1.

	// make the new leaf & populate it with the actual data you're trying to add
	n := new(polNode)
	n.data = add
	if remember {
		// flag this leaf as memorable via it's left pointer
		n.niece[0] = n // points to itself (mind blown)
	}
	// if add is forgetable, forget all the new nodes made
	var h uint8
	// loop until we find a zero; destroy tops until you make one
	for ; (p.numLeaves>>h)&1 == 1; h++ {
		// grab, pop, swap, hash, new
		leftTop := p.tops[len(p.tops)-1]                           // grab
		p.tops = p.tops[:len(p.tops)-1]                            // pop
		leftTop.niece, n.niece = n.niece, leftTop.niece            // swap
		nHash := Parent(leftTop.data, n.data)                      // hash
		n = &polNode{data: nHash, niece: [2]*polNode{&leftTop, n}} // new
		p.hashesEver++

		n.prune()

	}

	// the new tops are all the 1 bits above where we got to, and nothing below where
	// we got to.  We've already deleted all the lower tops, so append the new
	// one we just made onto the end.

	p.tops = append(p.tops, *n)
	p.numLeaves++
	return nil
}

// Hash and swap.  "grabPos" in rowdirt / hashdirt is inefficient because you
// descend to the place you already just decended to perfom swapNodes.

// rem2 deletes stuff from the pollard, using remtrans2
func (p *Pollard) rem2(dels []uint64) error {
	if len(dels) == 0 {
		return nil // that was quick
	}
	ph := p.height() // height of pollard
	nextNumLeaves := p.numLeaves - uint64(len(dels))

	// get all the swaps, then apply them all
	swaprows := remTrans2(dels, p.numLeaves, ph)

	wg := new(sync.WaitGroup)

	fmt.Printf(" @@@@@@ rem2 nl %d ph %d rem %v\n", p.numLeaves, ph, dels)
	var hashdirt []uint64
	fmt.Printf("start rem %s", p.toString())
	// swap all the nodes
	for h := uint8(0); h < ph; h++ {
		rowdirt := hashdirt
		hashdirt = []uint64{}
		for _, s := range swaprows[h] {
			if s.from == s.to {
				// TODO should get rid of these upstream
				continue
			}
			hn, err := p.swapNodes(s)
			if err != nil {
				return err
			}

			if hn.sib.niece[0].data == empty || hn.sib.niece[1].data == empty {
				fmt.Printf("swap %v hn empty data in sibling\n", s)
				// if the data's not there, that means we don't actually need
				// to hash it... (assuming that everything else is working
				// correctly)
				continue
			}

			// chop off first rowdirt (current row) if it's getting hashed
			// by the swap
			if len(rowdirt) != 0 &&
				(rowdirt[0] == s.to || rowdirt[0] == s.to^1) {
				fmt.Printf("%d in rowdirt, already got to from swap\n", s.to)
				rowdirt = rowdirt[1:]
			} else {
				fmt.Printf("rowdirt %v no match %d\n", rowdirt, s.to)
			}

			if hn != nil {
				fmt.Printf("giving hasher %d %x %x\n",
					s.to, hn.sib.niece[0].data[:4], hn.sib.niece[1].data[:4])
				// TODO some of these hashes are useless as they end up outside
				// the forest.
				// aside from TODO above, always hash the parent of swap "to"
				wg.Add(1)
				go hn.run(wg)
			}
			hashdirt = dirtify(hashdirt, swaprows, s.to, nextNumLeaves, h+2, ph)
		}
		// done with swaps for this row, now hashdirt
		// build hashable nodes from hashdirt
		for _, d := range rowdirt {
			hn, err := p.HnFromPos(d)
			if err != nil {
				return err
			}
			if hn == nil { // if d is a top
				fmt.Printf("hn is nil at pos %d\n", d)
				continue
			}
			fmt.Printf("dirting hasher %d %x %x\n",
				d, hn.sib.niece[0].data[:4], hn.sib.niece[1].data[:4])
			wg.Add(1)
			go hn.run(wg)
			hashdirt = dirtify(hashdirt, swaprows, d, nextNumLeaves, h+2, ph)
		}
		wg.Wait() // wait for all hashing to finish at end of each row
		fmt.Printf("done with row %d %s\n", h, p.toString())
	}

	fmt.Printf("pretop %s", p.toString())
	// set new tops
	nextTopPoss, _ := getTopsReverse(nextNumLeaves, ph)
	nexTops := make([]polNode, len(nextTopPoss))
	for i, _ := range nexTops {
		fmt.Printf("ntp grab top %d pos %d\n", i, nextTopPoss[i])
		ntpar, ntparsib, lr, err := p.grabPos2(nextTopPoss[i])
		if err != nil {
			return err
		}

		if ntpar == nil { // was already a top / overlap
			fmt.Printf("grabbed nil ntpar\n")
			nexTops[i] = p.tops[lr]
		} else { // node becoming a top, ntpar exists
			if ntparsib == nil {
				return fmt.Errorf("nexTops nil ntparsib")
				// nexTops[i].chop()
			}
			if ntparsib.niece[lr] == nil {
				return fmt.Errorf("nexTops nil ntparsib niece[%d]", lr)
			}
			fmt.Printf("non nil grabbed par %x parsib %x\n",
				ntpar.data[:4], ntparsib.data[:4])
			nexTops[i] = *ntparsib.niece[lr]
			if ntparsib.niece[lr^1] != nil {
				nexTops[i].niece = ntparsib.niece[lr^1].niece
			}
		}
		fmt.Printf("grab done %d %x\n", nextTopPoss[i], nexTops[i].data[:4])
	}

	p.numLeaves = nextNumLeaves
	reversePolNodeSlice(nexTops)
	p.tops = nexTops

	return nil
}

func (p *Pollard) HnFromPos(pos uint64) (*hashableNode, error) {
	par, parsib, _, err := p.grabPos2(pos)
	if err != nil {
		return nil, err
	}
	hn := new(hashableNode)

	hn.dest = par
	hn.sib = parsib
	return hn, nil
}

// dirtify adds to the next dirt row
func dirtify(dirt []uint64, swaps [][]arrow, pos, nl uint64, up2h, ph uint8) []uint64 {
	// is parent's parent in forest? if so, add *parent* to dirt
	parpar := upMany(pos, 2, ph)
	if !inForest(parpar, nl, ph) {
		// skip, UNLESS it moves to somewhere inside the forest range
		// due to the swaps in the next row up
		// TODO this is bad and inefficient as it may result in checking through
		// a LOT of a stuff for no reason.  Also, do I have to check ALL higher
		// rows instead of just the immediate higher row???  Fix / remove this
		// if possible.  Or at least profile to see how bad it is in practice;
		// maybe OOF parpars happen very rarely

		if up2h >= uint8(len(swaps)) {
			// fmt.Printf("%d parpar %d not in forest and no more swaps\n", pos, parpar)
			return dirt
		}

		var moves bool
		for _, a := range swaps[up2h] {
			if parpar == a.from {
				moves = true
				break
			}
		}
		if !moves {
			// fmt.Printf("%d parpar %d outside up2row %v\n", pos, parpar, swaps[up2h])
			return dirt
		}
		// fmt.Printf("%d parpar %d returns up2row %v\n", pos, parpar, swaps[up2h])

	}
	par := up1(pos, ph)
	if len(dirt) != 0 &&
		(dirt[len(dirt)-1] != pos || dirt[len(dirt)-1] != pos^1) {
		return dirt
	}

	dirt = append(dirt, par)
	fmt.Printf("ph %d nl %d %d parpar %d is in* forest, add %d\n",
		ph, nl, pos, parpar, par)
	return dirt

}

// swapNodes swaps the nodes at positions a and b.
// returns a hashable node with b, bsib, and bpar
func (p *Pollard) swapNodes(r arrow) (*hashableNode, error) {
	if !inForest(r.from, p.numLeaves, p.height()) ||
		!inForest(r.to, p.numLeaves, p.height()) {
		return nil, fmt.Errorf("swapNodes %d %d out of bounds", r.from, r.to)
	}
	fmt.Printf("swapNodes swapping a %d b %d\n", r.from, r.to)

	// TODO could be improved by getting the highest common ancestor
	// and then splitting instead of doing 2 full descents

	apar, aparsib, alr, err := p.grabPos2(r.from)
	if err != nil {
		return nil, err
	}
	bpar, bparsib, blr, err := p.grabPos2(r.to)
	if err != nil {
		return nil, err
	}

	// if aparsib == nil {
	// 	return nil, fmt.Errorf("swapNodes %v a nil parsib", r)
	// }
	// if bparsib == nil {
	// 	return nil, fmt.Errorf("swapNodes %v b nil parsib", r)
	// }

	// fmt.Printf("aparsib %x bparsib %x\n", aparsib.data[:4], bparsib.data[:4])

	if bpar == nil { // b is a top (can this happen...?)
		// TODO I don't think this can happen
		panic("why yes it can")
		// apar.niece[alr], p.tops[blr] = &p.tops[blr], *apar.niece[alr]
	}

	hn := new(hashableNode)
	// a is aparsib.niece[alr], a's sibling is aparsib.niece[alr^1]
	// b is bparsib.niece[blr], b's sibling is bparsib.niece[blr^1]

	if apar == nil { // a is a top, has no parent
		fmt.Printf("bpar %x\n", bpar.data[:4])
		fmt.Printf("bparsib %x\n", bparsib.data[:4])
		fmt.Printf("bparsib.[%d] %x\n", blr^1, bparsib.niece[blr^1].data[:4])
		fmt.Printf("p.tops[alr] %x\n", p.tops[alr].data[:4])

		if bparsib != nil && bparsib.niece[blr] != nil {
			fmt.Printf("\ttop swap\t %x %x\n",
				p.tops[alr].data[:4], bparsib.niece[blr].data[:4])
		}

		// ugh this is really weird an unintuitve
		bparsib.niece[blr].niece,
			bparsib.niece[blr^1].niece,
			p.tops[alr].niece =
			bparsib.niece[blr^1].niece,
			p.tops[alr].niece,
			bparsib.niece[blr].niece

		// why do you have to stash? something pointery...
		// yeah this works but direct swap doesn't because of something
		// involving pointers I bet.
		stash := p.tops[alr]
		p.tops[alr] = *bparsib.niece[blr]
		bparsib.niece[blr] = &stash
	} else { // normal swap, neither is a top
		fmt.Printf("normal swap\t")
		if r.from != r.to^1 {
			fmt.Printf("swap %x niece w %x niece\n",
				aparsib.niece[alr^1].data[:4], bparsib.niece[blr^1].data[:4])
			// swap sibling nieces if not siblings
			aparsib.niece[alr^1].niece, bparsib.niece[blr^1].niece =
				bparsib.niece[blr^1].niece, aparsib.niece[alr^1].niece
		}
		aparsib.niece[alr], bparsib.niece[blr] =
			bparsib.niece[blr], aparsib.niece[alr]
	}

	hn.dest = bpar
	hn.sib = bparsib

	if bparsib.niece[0] == nil || bparsib.niece[1] == nil {
		return nil, fmt.Errorf("pos %d %x bparsib nil niece", r.to, bparsib.data[:4])
	}

	fmt.Printf("hn dest %x sib %x\n", hn.dest.data[:4], hn.sib.data[:4])
	return hn, nil
}

// grabPos2 is like grabPos but simpler...?  Returns the PARENT of the thing
// you asked for, as well as the 0/1 uint8 of which it is (which is obvious
// as it's just pos &1.  BUT if the thing you asked for is a top, then it
// returns nil n as there is no parent, and the uint8 it returns is WHICH
// top it is.  So no error and nil n means get your own top
func (p *Pollard) grabPos2(pos uint64) (par, parsib *polNode, lr uint8, err error) {
	tree, branchLen, bits := detectOffset(pos, p.numLeaves)
	if (tree) >= uint8(len(p.tops)) {
		err = fmt.Errorf("grab2 %d not in forest", pos)
		return
	}
	if branchLen == 0 { // can't return a top's parent, so return which parent
		fmt.Printf("grab2 %d, is top. tree %d, bl %d bits %x\n", pos, tree, branchLen, bits)
		lr = tree
		return
	}
	fmt.Printf("grab2 %d, tree %d, bl %d bits %x\n", pos, tree, branchLen, bits)
	par, parsib = &p.tops[tree], &p.tops[tree]
	for h := branchLen - 1; h != 0; h-- { // go through branch
		lr = uint8(bits>>h) & 1
		fmt.Printf("h %d parsib %x lr %d\n", h, parsib.data[:4], lr)
		// if a sib doesn't exist, need to create it and hook it in
		if parsib.niece[lr^1] == nil {
			fmt.Printf("%x.niece[%d] not there, making\n", parsib.data[:4], lr^1)
			parsib.niece[lr^1] = new(polNode)
		}
		par, parsib = parsib.niece[lr^1], parsib.niece[lr]
		fmt.Printf("grab2 h %d now parsib %x\n", h, parsib.data[:4])
		if par == nil {
			err = fmt.Errorf("grab2 can't grab %d nil neice at height %d", pos, h)
			return
		}
	}
	lr = uint8(pos & 1) // kindof pointless but
	return
}

// grabPos is like descendToPos but simpler.  Returns the thing you asked for,
// as well as its sibling.  And an error if it can't get it.
// NOTE errors are not exhaustive; could return garbage without an error
func (p *Pollard) grabPos(
	pos uint64) (n, nsib *polNode, hn *hashableNode, err error) {
	tree, branchLen, bits := detectOffset(pos, p.numLeaves)
	// fmt.Printf("grab %d, tree %d, bl %d bits %x\n", pos, tree, branchLen, bits)
	n, nsib = &p.tops[tree], &p.tops[tree]
	for h := branchLen - 1; h != 255; h-- { // go through branch
		lr := uint8(bits>>h) & 1
		if h == 0 { // if at bottom, done
			hn = new(hashableNode)
			hn.dest = nsib // this is kind of confusing eh?
			hn.sib = n     // but yeah, switch siblingness
			n, nsib = n.niece[lr^1], n.niece[lr]
			if nsib == nil || n == nil {
				return // give up and don't make hashable node
			}
			// fmt.Printf("h%d n %x nsib %x npar %x\n",
			// 	h, n.data[:4], nsib.data[:4], npar.data[:4])
			return
		}
		// if a sib doesn't exist, need to create it and hook it in
		if n.niece[lr^1] == nil {
			n.niece[lr^1] = new(polNode)
		}
		n, nsib = n.niece[lr], n.niece[lr^1]
		// fmt.Printf("h%d n %x nsib %x npar %x\n",
		// 	h, n.data[:4], nsib.data[:4], npar.data[:4])
		if n == nil {
			// if a node doesn't exist, crash
			err = fmt.Errorf("grab %d nil neice at height %d", pos, h)
			return
		}
	}
	return // only happens when returning a top
	// in which case npar will be nil
}

// DescendToPos returns the path to the target node, as well as the sibling
// path.  Retruns paths in bottom-to-top order (backwards)
// sibs[0] is the node you actually asked for
func (p *Pollard) descendToPos(pos uint64) ([]*polNode, []*polNode, error) {
	// interate to descend.  It's like the leafnum, xored with ...1111110
	// so flip every bit except the last one.
	// example: I want leaf 12.  That's 1100.  xor to get 0010.
	// descent 0, 0, 1, 0 (left, left, right, left) to get to 12 from 30.
	// need to figure out offsets for smaller trees.

	if !inForest(pos, p.numLeaves, p.height()) {
		//	if pos >= (p.numLeaves*2)-1 {
		return nil, nil,
			fmt.Errorf("OOB: descend to %d but only %d leaves", pos, p.numLeaves)
	}

	// first find which tree we're in
	tNum, branchLen, bits := detectOffset(pos, p.numLeaves)
	//	fmt.Printf("DO pos %d top %d bits %d branlen %d\n", pos, tNum, bits, branchLen)
	n := &p.tops[tNum]
	if branchLen > 64 {
		return nil, nil, fmt.Errorf("dtp top %d is nil", tNum)
	}

	proofs := make([]*polNode, branchLen+1)
	sibs := make([]*polNode, branchLen+1)
	// at the top of the branch, the proof and sib are the same
	proofs[branchLen], sibs[branchLen] = n, n
	for h := branchLen - 1; h < 64; h-- {
		lr := (bits >> h) & 1

		sib := n.niece[lr^1]
		n = n.niece[lr]

		if n == nil && h != 0 {
			return nil, nil, fmt.Errorf(
				"descend pos %d nil neice at height %d", pos, h)
		}

		if n != nil {
			// fmt.Printf("target %d h %d d %04x\n", pos, h, n.data[:4])
		}

		proofs[h], sibs[h] = n, sib

	}
	//	fmt.Printf("\n")
	return proofs, sibs, nil
}

// toFull takes a pollard and converts to a forest.
// For debugging and seeing what pollard is doing since there's already
// a good toString method for  forest.
func (p *Pollard) toFull() (*Forest, error) {

	ff := NewForest()
	ff.height = p.height()
	ff.numLeaves = p.numLeaves
	ff.forest = make([]Hash, 2<<ff.height)
	if p.numLeaves == 0 {
		return ff, nil
	}

	//	for topIdx, top := range p.tops {
	//	}
	for i := uint64(0); i < (2<<ff.height)-1; i++ {
		_, sib, err := p.descendToPos(i)
		if err != nil {
			//	fmt.Printf("can't get pos %d: %s\n", i, err.Error())
			continue
			//			return nil, err
		}
		if sib[0] != nil {
			ff.forest[i] = sib[0].data
			//	fmt.Printf("wrote leaf pos %d %04x\n", i, sib[0].data[:4])
		}

	}

	return ff, nil
}

func (p *Pollard) toString() string {
	f, err := p.toFull()
	if err != nil {
		return err.Error()
	}
	return f.toString()
}

// equalToForest checks if the pollard has the same leaves as the forest.
// doesn't check tops and stuff
func (p *Pollard) equalToForest(f *Forest) bool {
	if p.numLeaves != f.numLeaves {
		return false
	}

	for leafpos := uint64(0); leafpos < f.numLeaves; leafpos++ {
		n, _, _, err := p.grabPos(leafpos)
		if err != nil {
			return false
		}
		if n.data != f.forest[leafpos] {
			fmt.Printf("leaf position %d pol %x != forest %x\n",
				leafpos, n.data[:4], f.forest[leafpos][:4])
			return false
		}
	}
	return true
}

// equalToForestIfThere checks if the pollard has the same leaves as the forest.
// it's OK though for a leaf not to be there; only it can't exist and have
// a different value than one in the forest
func (p *Pollard) equalToForestIfThere(f *Forest) bool {
	if p.numLeaves != f.numLeaves {
		return false
	}

	for leafpos := uint64(0); leafpos < f.numLeaves; leafpos++ {
		n, _, _, err := p.grabPos(leafpos)
		if err != nil || n == nil {
			continue // ignore grabPos errors / nils
		}
		if n.data != f.forest[leafpos] {
			fmt.Printf("leaf position %d pol %x != forest %x\n",
				leafpos, n.data[:4], f.forest[leafpos][:4])
			return false
		}
	}
	return true
}
