#include <stdio.h>
#include "gostr.h"
#include "libtsgo_cpp.h"

int main(void) {
    const char* my_ts_code = "import console from 'console'; console.log('Hello');";

    GoStr result1;
    result1.p = transpile((char*)"test.ts", (char*)my_ts_code, NULL, NULL);
    printf("\n%s\n", result1.p ? result1.p : "");
    GoStr_free(result1);

    return 0;
}
