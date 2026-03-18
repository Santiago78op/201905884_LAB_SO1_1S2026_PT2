KDIR := /home/julian/linux
PWD  := $(shell pwd)

obj-m += module_kernel.o

all:
	$(MAKE) -C $(KDIR) M=$(PWD) LLVM=1 modules

clean:
	$(MAKE) -C $(KDIR) M=$(PWD) clean