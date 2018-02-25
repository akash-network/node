# Deployments

## Usage

### Transaction

#### command
```
./photon deploy [filepath] -k master
```

##### returns
The 32 byte address of the deployment

### Query

#### command
```
./photon query deployment [address]
```

##### returns
A deployment object located at [address]

#### command
```
./photon query deployment
```

##### returns
All deployment objects
