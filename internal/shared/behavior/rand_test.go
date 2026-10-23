package behavior

import "math/rand/v2"

var r = testRand()

func testRand() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }
