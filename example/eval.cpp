#include <algorithm>
#include <chrono>
#include <iomanip>
#include <iostream>
#include <ostream>
#include <random>
#include <vector>

void fast_sort(std::vector<int>& v);

int main() {
    double score = 0;
    std::mt19937 rng(42);

    for(int len : {1, 2, 5, 10, 100, 1000, 10000, 100000, 1000000}) {
        int iters = 1e7 / len;
        std::vector<int> v(len), v_std(len);

        for(int it = 0; it < iters; it++) {
            for(int i = 0; i < len; i++) {
                v[i] = v_std[i] = rng();
            }

            fast_sort(v);
            std::sort(v_std.begin(), v_std.end());
            if(v != v_std) {
                std::cout << -1 << '\n';
                return 1;
            }
        }

        int64_t fast_time = 0;
        for(int it = 0; it < iters; it++) {
            for(int i = 0; i < len; i++) {
                v[i] = rng();
            }

            auto start = std::chrono::high_resolution_clock::now();
            fast_sort(v);
            auto end = std::chrono::high_resolution_clock::now();
            fast_time += std::chrono::duration_cast<std::chrono::nanoseconds>(end - start).count();
        }

        int64_t std_time = 0;
        for(int it = 0; it < iters; it++) {
            for(int i = 0; i < len; i++) {
                v[i] = rng();
            }

            auto start = std::chrono::high_resolution_clock::now();
            std::sort(v.begin(), v.end());
            auto end = std::chrono::high_resolution_clock::now();
            std_time += std::chrono::duration_cast<std::chrono::nanoseconds>(end - start).count();
        }

        score += static_cast<double>(fast_time) / static_cast<double>(std_time * 9);
    }

    std::cout << std::fixed << std::setprecision(8) << score << '\n';
    return 0;
}
