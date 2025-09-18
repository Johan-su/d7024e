FROM alpine:latest

# Add the commands needed to put your compiled go binary in the container and
# run it when the container starts.
#
# See https://docs.docker.com/engine/reference/builder/ for a reference of all
# the commands you can use in this file.
#
# In order to use this file together with the docker-compose.yml file in the
# same directory, you need to ensure the image you build gets the name
# "kadlab", which you do by using the following command:
#
# $ docker build . -t kadlab

COPY bin/kademlia_linux.exe .
RUN mv kademlia_linux.exe kademlia

ENTRYPOINT ["./kademlia", "-bip", "10.0.1.3", "-bid", "a353be5db2fcadc14b71b0ca900546c62f0edd9b"] 