#include <iostream>
#include <memory>
#include <string_view>
#include <cstdlib>
#include "libslim_tsgo.h"

struct GoStr {
    char* p;
    GoStr(char* p) : p(p) {}
    ~GoStr() { free(p); }
    std::string_view view() const { return p; }
};

int main() {
    std::string ts_code =
        "const v1: Vector3 = { x: 1, y: 2, z: 3 };\n"
        "const v2: Vector3 = { x: 4, y: 5, z: 6 };\n"
        "const v3 = add(v1, v2);\n"
        "const v4 = scale(v1, 2);\n"
        "console.log(dot(v3, v4));\n"
        "const v5: Vector3 = { x: 1, y: 2, z: 3 };\n"
        "const v6: Vector3 = { x: 4, y: 5, z: 6 };\n"
        "const v7 = add(v5, v6);\n"
        "const v8 = scale(v5, 2);\n"
        "console.log(dot(v7, v8));\n";

    //std::string my_ts_code = "import console from 'console'; console.log('Hello');";
    //GoStr result1(transpile(const_cast<char*>("test.ts"), const_cast<char*>(my_ts_code.c_str()), nullptr, nullptr));
    //std::cout << "\n" << result1.view() << std::endl;
    //GoStr result(transpile(const_cast<char*>("dir1/dir2/dir3/test.ts"), const_cast<char*>(my_ts_code.c_str()), const_cast<char*>("dist")));
    //GoStr result2(transpile(const_cast<char*>("point.ts"), const_cast<char*>(ts_code.c_str()), const_cast<char*>(dts_code.c_str()), nullptr));
    //GoStr result2(transpile(const_cast<char*>("point.ts"), const_cast<char*>(ts_code.c_str()), nullptr, nullptr));
    //std::cout << "\n" << result2.view() << std::endl;
    GoStr result3(build(const_cast<char*>("src"), const_cast<char*>("dist")));
    std::cout << "\n" << result3.view() << std::endl;
    return 0;
}
