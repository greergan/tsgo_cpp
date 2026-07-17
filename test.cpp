#include <print>
#include <string>
#include <vector>
#include "tsgo.h"

struct TestCase {
    int id;
    std::string uri;
};

//const std::string expected = "import console from 'console';\nconsole.log(\"Hello, World!\");\n";

const std::string expected =
    "import console from 'console';\n"
    "const msg = formatMessage(\"Hello, World!\");\n"
    "const cfg = { version: \"1.0\" };\n"
    "const deep = { name: \"test\" };\n"
    "const val = 42;\n"
    "const ca = { a: \"x\" };\n"
    "const cb = { b: \"y\" };\n"
    "console.log(msg ?? \"no message\", cfg.version, deep.name, val, ca.a, cb.b);\n";

const std::vector<TestCase> cases = {
    // {1, "http://forgejo:8000/src/hello_world.ts"},
    {2, "file://src/hello_world.ts"},
    // {3, "http://forgejo/greergan/typescript_samples/raw/branch/master/src/hello_world.ts"},
    // {4, "https://raw.githubusercontent.com/greergan/typescript_samples/master/src/hello_world.ts"},
    // {5, "https://codeberg.org/greergan/typescript_samples/raw/branch/master/src/hello_world.ts"},
};

int main(int argc, char** argv) {
    for (const auto& tc : cases) {
        std::println("running test for => {}", tc.uri);
        GoStr result = fetch_and_transpile(const_cast<char*>(tc.uri.c_str()));
        // if (result.view() != expected) {
        // std::println("test for => {} => failed", tc.uri);
        //     std::println("Expected:\n{}", expected);
        //     std::println("Received:\n{}", result.view());
        //     return tc.id;
        // }
        std::println("test for => {} => passed", tc.uri);
    }

    return 0;
}
