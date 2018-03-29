FROM busybox:glibc

COPY ./akash-docker  ./akash
COPY ./akashd-docker ./akashd

EXPOSE 46656 46657 46658
