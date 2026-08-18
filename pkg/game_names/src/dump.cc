#include <dlfcn.h>
#include <cstdio>
#include <cstdlib>
#include <cassert>
#include <cstring>

struct Entry {
		const char *key;
		const char *value;
};

int main(int argc, char *argv[]) {
		void *handle = dlopen("./libgamename.so", RTLD_NOW);
		assert(handle != nullptr);

		auto *ch = reinterpret_cast<Entry *>(dlsym(handle, "s_mamedb_ch"));
		auto *en = reinterpret_cast<Entry *>(dlsym(handle, "s_mamedb_en"));
		assert(ch != nullptr && en != nullptr);

		if(argc==1 || (argc==2 && strcmp(argv[1], "zh")==0)) {
				for (int i=0; ch[i].key != nullptr ; i++) {
						printf("%s\t%s\n", ch[i].key, ch[i].value);
				}
		}

		if(argc==1 || (argc==2 && strcmp(argv[1], "en")==0)) {
				for (int i = 0; en[i].key != nullptr; i++) {
						printf("%s\t%s\n", en[i].key, en[i].value);
				}
		}

		dlclose(handle);
		return 0;
}
