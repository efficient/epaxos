# assume EPaxos source code has been clone to efolder
efolder=${HOME}/go/src/epaxos
profile=${HOME}/.bash_profile # .bash_profile for MacOS but .profile for Ubuntu?

touch ${profile} # create one if profile does not exist

# https://stackoverflow.com/a/1397020
# for go build to work; for running Shell code anywhere; for running EPaxos' binaries anywhere
if [[ ":${GOPATH}:" != *":${efolder}:"* ]]; then
  # echo 'export GOPATH=${GOPATH}':"${efolder}" >>${profile}
  # there are some unknown issue prevents the server code to compile if the GOPATH variable set in ${profile} has more than one entry
  # as a makeshift, I will do this instead:
  export GOPATH=${GOPATH}:${efolder}
  source ${profile}
fi
if [[ ":${PATH}:" != *":${efolder}:"* ]]; then
  echo 'export PATH=${PATH}':"${efolder}" >>${profile}
  chmod 777 ${efolder}/*.sh
  source ${profile}
fi
if [[ ":${PATH}:" != *":${efolder}/bin:"* ]]; then
  echo 'export PATH=${PATH}':"${efolder}/bin" >>${profile}
  source ${profile}
fi

cd ${efolder}

rm -rf ${efolder}/bin/*
go build -o ${efolder}/bin/master master # emaster -N=3
echo "Built Master"
go build -o ${efolder}/bin/server server
echo "Built Server"
go build -o ${efolder}/bin/client client
echo "Built Client"