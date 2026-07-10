#include <iostream>
#include <cstdlib> // Required for free()
#include "libtsgo.h"

int main() {
    std::string my_ts_code = "console.log('Hello'); export {};";

    // Pass using const_cast
    char* js_result = TranspileAndCheckTS(const_cast<char*>(my_ts_code.c_str()));

    if (js_result != nullptr) {
        std::cout << "Transpiled JS: " << js_result << std::endl;

        // Free the memory allocated by Go
        free(js_result);
    }

    return 0;
}
