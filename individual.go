package cgp

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// A Gene contains the index of a CGPFunction, a constant and connections to the
// inputs for the function.
type gene struct {
	Function    int
	Constant    float64
	Connections []int
}

// Mutate replaces function, constant or connections of a Gene with a random
// valid value
func (g *gene) Mutate(position int, options *CGPOptions) {

	toMutate := options.Rand.Intn(2 + len(g.Connections))

	if toMutate == 0 {
		g.Function = options.Rand.Intn(len(options.FunctionList))
		return
	}

	if toMutate == 1 {
		g.Constant = options.RandConst(options.Rand)
		return
	}

	g.Connections[toMutate-2] = options.Rand.Intn(position)
}

// An Individual represents the genetic code of an evolved program. It contains
// function genes and output genes and can hold the fitness of the evolved
// program.
type Individual struct {
	Genes       []gene      // The function genes
	Outputs     []int       // The output genes
	Options     *CGPOptions // A pointer to the CGPOptions. Necessary to retrieve e.g. the mutation rate.
	Fitness     float64     // The fitness of the individual
	activeGenes []bool      // Which genes are active (contribute to program output)
	cacheID     string      // Programs with the same CacheID will behave identically
}

// NewIndividual creates a random valid program with the options as specified.
func NewIndividual(options *CGPOptions) (ind Individual) {
	ind.Options = options
	ind.Fitness = math.Inf(1)
	ind.Genes = make([]gene, options.NumGenes)
	ind.Outputs = make([]int, options.NumOutputs)

	for i := range ind.Genes {
		ind.Genes[i].Function = options.Rand.Intn(len(options.FunctionList))
		ind.Genes[i].Constant = options.RandConst(options.Rand)
		ind.Genes[i].Connections = make([]int, options.MaxArity)
		for j := range ind.Genes[i].Connections {
			ind.Genes[i].Connections[j] = options.Rand.Intn(options.NumInputs + i)
		}
	}

	for i := range ind.Outputs {
		ind.Outputs[i] = options.Rand.Intn(options.NumInputs + options.NumGenes)
	}

	return
}

// Mutate returns a mutated copy of the Individual.
func (ind Individual) Mutate() (mutant Individual) {
	// Copy the parent individual
	mutant.Fitness = math.Inf(1)
	mutant.Options = ind.Options
	mutant.Genes = make([]gene, ind.Options.NumGenes)
	mutant.Outputs = make([]int, ind.Options.NumOutputs)
	copy(mutant.Genes, ind.Genes)
	copy(mutant.Outputs, ind.Outputs)

	numMutations := ind.Options.MutationRate *
		float64((ind.Options.NumGenes*(2+ind.Options.MaxArity))+ind.Options.NumOutputs)
	if numMutations < 1 {
		numMutations = 1
	}

	for numMutations > 0 {
		toMutate := ind.Options.Rand.Intn(mutant.Options.NumGenes + mutant.Options.NumOutputs)

		if toMutate < mutant.Options.NumGenes {
			mutant.Genes[toMutate].Mutate(toMutate+mutant.Options.NumInputs, mutant.Options)
		} else {
			mutant.Outputs[toMutate-mutant.Options.NumGenes] =
				ind.Options.Rand.Intn(mutant.Options.NumInputs + mutant.Options.NumGenes)
		}

		numMutations--
	}

	return
}

// Recursively marks genes as active
func (ind *Individual) markActive(gene int) {
	if ind.activeGenes[gene] {
		return
	}

	ind.activeGenes[gene] = true
	//TODO - only mark num arity conns active?
	//for _, conn := range ind.Genes[gene-ind.Options.NumInputs].Connections {
	//	ind.markActive(conn)
	//}

	arity := ind.Options.FunctionList[ind.Genes[gene-ind.Options.NumInputs].Function].Arity
	for i := 0; i < arity; i++ {
		ind.markActive(ind.Genes[gene-ind.Options.NumInputs].Connections[i])
	}

}

func (ind *Individual) determineActiveGenes() {
	// Check if we already did this
	if len(ind.activeGenes) != 0 {
		return
	}

	//MCC ind.activeGenes = make([]bool, ind.Options.NumInputs+ind.Options.NumGenes+ind.Options.NumOutputs)
	ind.activeGenes = make([]bool, ind.Options.NumInputs+ind.Options.NumGenes)

	// Mark inputs as Active
	for i := 0; i < ind.Options.NumInputs; i++ {
		ind.activeGenes[i] = true
	}

	// Recursively mark active genes beginning from the outputs
	for _, conn := range ind.Outputs {
		ind.markActive(conn)
	}
}

// CacheID returns the functional ID of ind. Two individuals that have the same
// CacheID are guaranteed to compute the same function. Note that individuals
// that differ in their inactive genes but are identical in their active genes
// will have the same CacheID.
func (ind *Individual) CacheID() string {
	if len(ind.cacheID) != 0 {
		return ind.cacheID
	}

	ind.determineActiveGenes()

	h := md5.New()
	for i, g := range ind.Genes {
		if ind.activeGenes[i+ind.Options.NumInputs] {
			binary.Write(h, binary.LittleEndian, g.Function)
			binary.Write(h, binary.LittleEndian, g.Constant)
			for _, c := range g.Connections {
				binary.Write(h, binary.LittleEndian, c)
			}
		}
	}
	for _, o := range ind.Outputs {
		binary.Write(h, binary.LittleEndian, o)
	}

	ind.cacheID = hex.EncodeToString(h.Sum(nil))

	return ind.cacheID
}

// Run executes the evolved program with the given input.
func (ind Individual) Run(input []float64) []float64 {
	if len(input) != ind.Options.NumInputs {
		panic("Individual.Run() was called with the wrong number of inputs")
	}

	ind.determineActiveGenes()

	nodeOutput := make([]float64, ind.Options.NumInputs+ind.Options.NumGenes)
	for i := 0; i < ind.Options.NumInputs; i++ {
		nodeOutput[i] = input[i]
	}

	for i := 0; i < ind.Options.NumGenes; i++ {
		if !ind.activeGenes[i+ind.Options.NumInputs] {
			continue
		}

		functionInput := make([]float64, 1+ind.Options.MaxArity)
		functionInput[0] = ind.Genes[i].Constant
		for j, c := range ind.Genes[i].Connections {
			functionInput[j+1] = nodeOutput[c]
		}

		functionOutput := ind.Options.FunctionList[ind.Genes[i].Function].Eval(functionInput)
		if math.IsNaN(functionOutput) {
			ind.Genes[i].Function = 0
			functionOutput = ind.Options.FunctionList[ind.Genes[i].Function].Eval(functionInput)
		}
		nodeOutput[i+ind.Options.NumInputs] = functionOutput

	}

	output := make([]float64, 0, ind.Options.NumOutputs)
	for _, o := range ind.Outputs {
		output = append(output, nodeOutput[o])
	}

	return output
}

func (ind Individual) parse(result string, index int) string {

	if index < ind.Options.NumInputs {
		result += fmt.Sprintf("x%d", index)
		return result
	}

	name := ind.Options.FunctionList[ind.Genes[index-ind.Options.NumInputs].Function].Name
	arity := ind.Options.FunctionList[ind.Genes[index-ind.Options.NumInputs].Function].Arity
	conn := ind.Genes[index-ind.Options.NumInputs].Connections

	if name == "const" {

		result += fmt.Sprintf("%f", ind.Genes[index-ind.Options.NumInputs].Constant)

	} else if name == "add" {

		result += "("
		result = ind.parse(result, conn[0])
		for i := 1; i < arity; i++ {
			result += "+"
			result = ind.parse(result, conn[i])
		}
		result += ")"

	} else if name == "sub" {

		result += "("
		result = ind.parse(result, conn[0])
		for i := 1; i < arity; i++ {
			result += "-"
			result = ind.parse(result, conn[i])
		}
		result += ")"

	} else if name == "mul" {

		result += "("
		result = ind.parse(result, conn[0])
		for i := 1; i < arity; i++ {
			result += "*"
			result = ind.parse(result, conn[i])
		}
		result += ")"

	} else if name == "div" {

		result += "("
		result = ind.parse(result, conn[0])
		for i := 1; i < arity; i++ {
			result += "/"
			result = ind.parse(result, conn[i])
		}
		result += ")"
	} else {

		result += fmt.Sprintf("%s(", name)
		for i := 0; i < arity; i++ {
			result = ind.parse(result, conn[i])
			if i < arity-1 {
				result += ","
			}
		}
		result += ")"
	}
	return result
}

// Expr -
func (ind Individual) Expr() string {

	ind.determineActiveGenes()
	/*
		for i := 0; i < ind.Options.NumInputs; i++ {
			fmt.Printf("%d: x%d \n", i, i)
		}

		for i := 0; i < ind.Options.NumGenes; i++ {
			if ind.activeGenes[ind.Options.NumInputs+i] {
				name := ind.Options.FunctionList[ind.Genes[i].Function].Name
				fmt.Printf("%d: %s %v %f active=%v\n", ind.Options.NumInputs+i, name, ind.Genes[i].Connections, ind.Genes[i].Constant, ind.activeGenes[ind.Options.NumInputs+i])
			}
		}

		for i, conn := range ind.Outputs {
			fmt.Printf("output%d: %d \n", i, conn)
		}
	*/
	result := ""

	for o := 0; o < ind.Options.NumOutputs; o++ {
		result += "\n"
		if ind.Options.NumInputs == 0 {
			result += "f()="
		} else {
			result += fmt.Sprintf("f%d(x0", o)
			for i := 1; i < ind.Options.NumInputs; i++ {
				result += fmt.Sprintf(",x%d", i)
			}
			result += fmt.Sprintf(")=")
		}
		result = ind.parse(result, ind.Outputs[o])
	}
	return result
}
