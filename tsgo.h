#ifndef TSGO_H
#define TSGO_H
#ifdef __cplusplus
#include <string_view>
#include <cstdlib>

struct GoStr {
    char* p = nullptr;
    GoStr() = default;
    GoStr(char* ptr) : p(ptr) {}
    GoStr(const GoStr&) = delete;
    GoStr& operator=(const GoStr&) = delete;
    GoStr(GoStr&& other) noexcept : p(other.p) { other.p = nullptr; }
    GoStr& operator=(GoStr&& other) noexcept { if(this != &other) { free(p); p = other.p; other.p = nullptr; } return *this; }
    ~GoStr() { free(p); }
    std::string_view view() const { return p ? p : ""; }
};

#else
#include <stdlib.h>

typedef struct {
    char* p;
} GoStr;

static inline void GoStr_free(GoStr s) { free(s.p); }

#endif
#endif
