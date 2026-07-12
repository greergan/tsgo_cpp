all: slim_tsc_stub

libtsgo.a:
	cp -r ../typescript-bridge/types types
	go build -buildmode=c-archive -o libtsgo.a tsgo_bridge.go

slim_tsc_stub: clean libtsgo.a
	g++ -o slim_tsc_stub main.cpp libtsgo.a -lpthread -ldl
	./slim_tsc_stub

clean:
	rm -rf libtsgo.a libtsgo.h slim_tsc_stub dist types
	clear
