#ifndef TSGO_H
#define TSGO_H
#ifdef __cplusplus
#include <string_view>
#include <cstdlib>

struct GoStr {
    char* p;
    GoStr(char* ptr) : p(ptr) {}
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
