package utreexo

import (
	"fmt"
	"testing"
)

// test cases for TestTopDown:
// add 4, remove [0]
// add 5, remove [0, 3]
// add 8, remove [0, 2]
// add 8, remove [0, 2, 4, 6]

func TestTopDown(t *testing.T) {

	// mv, stash := removeTransform([]uint64{1}, 16, 4)
	// fmt.Printf("mv %v, stash %v\n", mv, stash)
	// arrows := mergeAndReverseArrows(mv, stash)
	// td := topDown(arrows, 4)
	// fmt.Printf("td %v\n", td)

	//  these should stay the same
	fup := NewForest()   // bottom up modified forest
	fdown := NewForest() // top down modified forest

	adds := make([]LeafTXO, 16)
	for j := range adds {
		adds[j].Hash[1] = uint8(j)
		adds[j].Hash[3] = 0xcc
	}

	_, err := fup.Modify(adds, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fdown.Modify(adds, nil)
	if err != nil {
		t.Fatal(err)
	}
	//initial state
	fmt.Printf(fup.toString())

	dels := []uint64{12}

	err = fup.removev3(dels) // was v2
	if err != nil {
		t.Fatal(err)
	}

	err = fdown.removev3(dels)
	if err != nil {
		t.Fatal(err)
	}

	upTops := fup.GetTops()
	downTops := fdown.GetTops()

	fmt.Printf("up nl %d %s", fup.numLeaves, fup.toString())
	fmt.Printf("down nl %d %s", fdown.numLeaves, fdown.toString())

	if len(upTops) != len(downTops) {
		t.Fatalf("tops mismatch up %d down %d\n", len(upTops), len(downTops))
	}
	for i, _ := range upTops {
		fmt.Printf("up %04x down %04x ", upTops[i][:4], downTops[i][:4])
		if downTops[i] != upTops[i] {
			t.Fatalf("forest mismatch, up %x down %x",
				upTops[i][:4], downTops[i][:4])
		}
	}

}

func TestRandTopDown(t *testing.T) {

	// mv, stash := removeTransform([]uint64{1}, 16, 4)
	// fmt.Printf("mv %v, stash %v\n", mv, stash)
	// arrows := mergeAndReverseArrows(mv, stash)
	// td := topDown(arrows, 4)
	// fmt.Printf("td %v\n", td)
	numAdds := 1005

	for b := 1; b < 1000; b++ {
		numDels := b
		//  these should stay the same
		fup := NewForest()   // bottom up modified forest
		fdown := NewForest() // top down modified forest

		delMap := make(map[uint64]bool)
		adds := make([]LeafTXO, numAdds)
		for j := range adds {
			adds[j].Hash[1] = uint8(j)
			adds[j].Hash[3] = 0xcc
			delMap[uint64(j)] = true
		}

		_, err := fup.Modify(adds, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fdown.Modify(adds, nil)
		if err != nil {
			t.Fatal(err)
		}

		//initial state
		fmt.Printf(fup.toString())

		var k int
		dels := make([]uint64, numDels)
		for i, _ := range delMap {
			dels[k] = i
			k++
			if k >= numDels {
				break
			}
		}

		err = fup.removev3(dels) // was v2b
		if err != nil {
			t.Fatal(err)
		}
		err = fdown.removev3(dels)
		if err != nil {
			t.Fatal(err)
		}

		upTops := fup.GetTops()
		downTops := fdown.GetTops()

		fmt.Printf("up nl %d %s", fup.numLeaves, fup.toString())
		fmt.Printf("down nl %d %s", fdown.numLeaves, fdown.toString())

		if len(upTops) != len(downTops) {
			t.Fatalf("tops mismatch up %d down %d\n", len(upTops), len(downTops))
		}
		for i, _ := range upTops {
			fmt.Printf("up %04x down %04x ", upTops[i][:4], downTops[i][:4])
			if downTops[i] != upTops[i] {
				t.Fatalf("forest mismatch, up %x down %x",
					upTops[i][:4], downTops[i][:4])
			}
		}
	}
}

func TestExTwin(t *testing.T) {

	fmt.Printf("%d\n", topPos(15, 0, 4))

	dels := []uint64{0, 1, 2, 3, 9}

	parents, dels := ExTwin2(dels, 4)

	fmt.Printf("parents %v dels %v\n", parents, dels)
}

func TestTopUp(t *testing.T) {

	fmt.Printf(BinString(15))

	rows := make([][]uint64, 5)
	rows[0] = []uint64{12}
	rows[1] = []uint64{21}
	// rows[2] = []uint64{23}
	topUp(rows, 4)

	fmt.Printf("%v\n", rows)
}

func TestGetTop(t *testing.T) {

	nl := uint64(11)
	h := uint8(1)
	top := topPos(nl, h, 4)

	fmt.Printf("%d leaves, top at h %d is %d\n", nl, h, top)
}
