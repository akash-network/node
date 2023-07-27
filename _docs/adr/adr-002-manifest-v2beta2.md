# ADR 002: Manifest v2beta2

## Changelog

* 2023/07/26: Initial implementation @troian

## Status

InProgress

## Brief
This manifest is using `v1beta3 API`

## Version

Manifest version is SHA256 of sorted json. Check reference [Go implementation](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/manifest.go#L53)

## Validation
[Reference implementation](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/manifest.go#L28C1-L28C1)

### Global
- Total amount of global service must be [> 0](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/groups.go#L29)

### Groups
- Group names must be [unique](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/groups.go#L22C22-L22C22)

### Group
- Group must have at least [one service](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/group.go#L41)
- Services must be sorted in ascending order [by name](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/group.go#L45)

#### Service
- `Name` must [not be empty](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/service.go#L14)
- `Name` must match regex `^[a-z]([-a-z0-9]*[a-z0-9])?$`
- `Image` must not be empty

##### Env var
Env must be in format `NAME<=VALUE>`, where `NAME` is mandatory, and value including `=` is optional

Name must comply with regex `^[-._a-zA-Z][-._a-zA-Z0-9]*$`

##### Expose
Expose list must be sorted. Reference [implementation](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/serviceexposes.go#L13..L41) 
Sort priorities are:
1. Name
2. Port
3. Proto
4. Global

Each expose member:
`Port` - `0 > port > 65535`
`Proto` - TCP or UDP
`Hosts` - each host must:
    - len <= 255 characters (UTF-8)
    - complies with **RFC 1123**

## CheckAgainstDeployment

[Reference implementation](https://github.com/akash-network/akash-api/blob/60498f7c84cfef78ebbfce97a818bf7610c94805/go/manifest/v2beta2/manifest.go#L47)
