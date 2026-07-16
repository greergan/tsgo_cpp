#include <print>
#include <string>
#include "tsgo.h"

int main(int argc, char** argv) {
    std::string expected = "import console from 'console';\nconsole.log(\"Hello, World!\");\n";

    std::string uri1 = "file://../typescript_samples/src/hello_world.ts";
    GoStr result1 = fetch_and_transpile(const_cast<char*>(uri1.c_str()));
    if(result1.view() != expected) {
        std::println("Test 1 Failed");
        std::println("Expected:\n{}", expected);
        std::println("Received:\n{}", result1.view());
        return 1;
    }

    std::string uri2 = "http://forgejo/greergan/typescript_samples/raw/branch/master/src/hello_world.ts";
    GoStr result2 = fetch_git_and_transpile(const_cast<char*>(uri2.c_str()));

    if(result2.view() != expected) {
        std::println("Test 2 Failed");
        std::println("Expected:\n{}", expected);
        std::println("Received:\n{}", result2.view());
        return 1;
    }

    std::string uri3 = "https://raw.githubusercontent.com/greergan/typescript_samples/master/src/hello_world.ts";
    GoStr result3 = fetch_git_and_transpile(const_cast<char*>(uri3.c_str()));

    if(result3.view() != expected) {
        std::println("Test 3 Failed");
        std::println("Expected:\n{}", expected);
        std::println("Received:\n{}", result3.view());
        return 1;
    }

    std::string uri4 = "http://codeberg.org/greergan/typescript_samples/raw/branch/master/src/hello_world.ts";
    GoStr result4 = fetch_git_and_transpile(const_cast<char*>(uri4.c_str()));

    if(result4.view() != expected) {
        std::println("Test 4 Failed");
        std::println("Expected:\n{}", expected);
        std::println("Received:\n{}", result4.view());
        return 1;
    }
    return 0;
}
