<h1 align="center">
  tinyevolve
</h1>

This is an evolutionary agent similar to AlphaEvolve. It takes some (or none) base solutions to some problem and evolves them using LLM to maximize some score function.

Existing implementations of AlphaEvolve out there contain complex heuristics that may or may not work for a specific problem. Contrary, tinyevolve is designed to be so simple that it can be completely understood by user and modified to fit your specific needs. Though it still has sane defaults to work good enough without extra configuration. The whole implementation is around 300 lines of Go code.

## How it works

Each solution is represented by a file with code. On each iteration, tinyevolve:

1. Takes some solution from a set of existing solutions as a base (see `getBaseSolutionID`)
2. Asks LLM to improve it, using template prompt from config, code of base solution, its score and comment from evaluator
3. Evaluates the result using command from config (`eval.command`), appending path to new solution file to the arguments. The command should print float score in a first line of stdout, and the rest of the output is treated as evaluator comment

tinyevolve stores the solutions in a data directory. For each solution, it also stores metadata in a `.meta` file, which contains solution score and evaluator comment. If some solutions have no corresponding `.meta` file, evaluator will be run for them to generate it. You can stop evolution at any moment and continue without losing the progress by just passing the same data directory.

## Example

In the example (see `example` directory), we try to find the fastest sorting algorithm for an array of integers in C++. We define a function `fast_sort` that we are trying to optimize and write its initial implementation:

```cpp
#include <algorithm>
#include <vector>

void fast_sort(std::vector<int> &v) {
    std::sort(v.begin(), v.end());
}
```

We save it in `data` directory as `initial.cpp`. On startup, tinyevolve will run evaluator on it and generate `.meta` file. Adding initial solution is optional, without it tinyevolve will just start from empty solution and LLM (with proper prompt) will generate initial solutions by itself.

Then we define a score function that we want to maximize - an average performance improvement over `std::sort` on different input sizes: 1, 2, 5, 10, 100, 1000, 10000, 100000, 1000000. We write main file that is compiled alongside with `fast_sort` implementation. It benchmarks `fast_sort` and prints result in stdout: score if everything is OK and `-1` if implementation gives wrong answer.

We also need a program that will compile and run our code and return evaluation result. For this we write a shell script that takes path to solution code file as the only argument, evaluates it, prints in stdout score value on the first line and evaluator comment on the rest. Evaluator comment tells what's exactly went wrong with solution: if it's compilation error, it will write error message from the compiler, if evaluation exceeded time limit, it will say so.

To run the example, you'll need to install C++ and Go compiler. Then build tinyevolve with:

```
go build .
```

Specify your LLM's OpenAI compatible endpoint, model and API key in `examples/config.toml`. Run evolution with this command (in `examples` directory):

```
../tinyevolve --config config.toml --data data --iterations 100
```

