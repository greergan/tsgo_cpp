#include <iostream>
#include <memory>
#include <string_view>
#include <cstdlib>
#include "libtsgo.h"

struct GoStr {
    char* p;
    GoStr(char* p) : p(p) {}
    ~GoStr() { free(p); }
    std::string_view view() const { return p; }
};

int main() {
    std::string my_ts_code = "import console from 'console'; console.log('Hello');";

    GoStr result(transpile(const_cast<char*>("test.ts"), const_cast<char*>(my_ts_code.c_str()), nullptr));
    //GoStr result(transpile(const_cast<char*>("dir1/dir2/dir3/test.ts"), const_cast<char*>(my_ts_code.c_str()), const_cast<char*>("dist")));

    std::cout << result.view() << std::endl;

    return 0;
}
