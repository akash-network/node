# The Akash Testnet

### About the testnet
The Akash testnet is a fully-functioning decentralized cloud, with support for requesting, deploying, and paying for cloud deployments. Server capacity is being kindly provided by Packet, the world's leading bare-metal provider. Access is **free** for registered users. As a free service, capacity is tightly managed, so please treat testnet capacity as the scarce community resource it is.  In other words, please play nicely in our sandbox.


We want your feedback!  Please message us [on Telegram](https://t.me/AkashNW) with any and all of your feedback.  We're putting our in-progess platform out there so that we can get real-world feedback, so don't be shy!


Finally, some warnings. The Akash testnet is at an alpha-level stage of development and so **not intended for production use.**  New functionality and capacity is being added constantly, but is always presented to you as-is, so use at your own risk. Use is at our discretion and we reserve the right to bring down deployments or to re-initialize the chain at any time for any reason.

### Intended users
Fundamentally, the Akash testnet is a deployment platform with a CLI and intended for relatively sophisticated users.  If you are comfortable managing instances via the AWS API and can build and deploy a Docker container, you will find the Akash testnet easy to use.  If not, please feel free to give it a shot, but you might find it confusing.

### Documentation
Full usage docs may be found at https://github.com/ovrclk/akash/blob/master/_docs/testnet/client-usage.md

### Installing the Akash client
The Akash client has been tested on Linux and MacOS. It *should* also work on Windows, but we haven't tested it - let us know if you try!

Instructions for installing the Akash client binary may be found here at https://github.com/ovrclk/akash/#installing.

### Getting help
First of course, [RTFM](https://github.com/ovrclk/akash/blob/master/_docs/testnet/client-usage.md), then please feel free to ask questions in our Telegram channel https://t.me/AkashNW.


## Getting started
The testnet supports this basic workflow:
 1. Register and receive testnet tokens
 1. Deploy your image
 1. Close your deployment (i.e. deprovision containers)

The sections below describe each step.


### Register for the testnet
You must register with Akash to request access to the testnet. This is a one-time only action.  After registering, you will receive a set of Akash testnet tokens **Please note that testnet tokens are only usable on the Akash testnet and have no market value.**


To register:
   1. Go to https://akash.network/testnet/ and follow the instructions.
   1. You'll immediately receive a confirmation email.  Click the link inside to confirm your email and request testnet tokens.
   1. After review by our staff, we will send tokens to your address and confirm with another email.

If you wish to receive another token allocation, repeat these steps and we will happily consider your request


### Deploy your image
In this step, you actually use the testnet to deploy an image, paying with your testnet tokens.

#### 1. Check your balance
```
$ akash key list #returns your key names and values
$ akash query account [key value] #returns your balance
```
For example:
```
$ akash key list
my-key-name 4b5446b97930b1885d11550cb2b277b6fee8e3ce

$ akash query account 4b5446b97930b1885d11550cb2b277b6fee8e3ce
{
  "address": "4b5446b97930b1885d11550cb2b277b6fee8e3ce",
  "balance": 420,
  "nonce": 1
}
```
#### 2. Download the sample deployment file and modify it if desired. 
The sample deployment file specifies a small webapp container running a simple demo site we created.  [You may download it here](testnet-deployment.yml).


You may use the sample deployment file as-is or modify it for your own needs as desscribed in our [SDL (Stack Definition Language) documentation](../sdl.md). A typical modification would be to reference your own image instead of our demo app image.  Note that you are limited in the amount of testnet resources you may request. Please see the [Limitations section](#testnet-limitations).

#### 3. Create a new deployment
In this step, you post your deployment, the Akash marketplace matches you with a provider via auction, and your image is deployed.
 ```
 $ akash key list
 $ akash deploy <deployment file path> -k <key name> #creates and sends the deployment
 ```
 The client will print the deployment id, bid, lease, and deployment data to console, for example:
 ```
$ akash deployment create ./testnet-deployment.yml -k my-key-name
66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357
Waiting...
Group 1/1 Fulfillment: 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc [price=11]
Group 1/1 Fulfillment: 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/a8954503bdd62134bf691c954d4eba3099952424ed708c7b69afeecaa8f9b38f [price=13]
Group 1/1 Lease: 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc [price=11]
Sending manifest to http://sjc.147.75.70.13.aksh.io...
Service URIs for provider: 38323234653134663930336132653133366136333632353237623139663131393335313937313735636236393938313934303933336161303434353961326139
	webapp: webapp.a138530f21e98e88bfc449d6736798fbe5130fa99b748d7aeb5d08b15e326cb8.147.75.70.13.aksh.io
```
The lease is returned in the form [deployment id]/[deployment group number]/[order number]/[provider address]. The public URL to each deployed service is also returned - you may use it as is or direct your own DNS to it.

You may also query your leases with `akash query lease`. For example:
```
$ akash query lease
{
  "items": [
    {
      "id": {
        "deployment": "66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357",
        "group": 1,
        "order": 2,
        "provider": "49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc"
      },
      "price": 11
    }
  ]
}
```
The price of your deployment is transferred from your account every second.

#### 4.  Access your deployed application in whatever way makes sense to you
You may also view your application logs with `akash logs <service name> <lease>`. For example, given a service named `webapp` and the lease above:

```
$ ./akash logs webapp 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357/1/2/49877504638723665f08dd57c2b0fbae79bd2abf65fe0d397e20880953b9befc -l 1 -f
[webapp-58c8984dfd-nbf6g]  2018-07-04T01:57:55.141522165Z 172.17.0.5 - - [04/Jul/2018:01:57:55 +0000] "GET /images/favicon.png HTTP/1.1" 200 1825 "http://hello.192.168.99.100.nip.io/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.99 Safari/537.36" "192.168.99.1"
[webapp-58c8984dfd-nbf6g]  2018-07-04T01:57:57.255819449Z 172.17.0.5 - - [04/Jul/2018:01:57:57 +0000] "GET /images/favicon.png HTTP/1.1" 200 1825 "http://hello.192.168.99.100.nip.io/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.99 Safari/537.36" "192.168.99.1"
[webapp-58c8984dfd-nbf6g]  2018-07-04T02:03:04.221319604Z 172.17.0.5 - - [04/Jul/2018:02:03:04 +0000] "GET / HTTP/1.1" 304 0 "-" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.99 Safari/537.36" "192.168.99.1"
```

### Close your deployment
When you are done with your application, close the deployment. This will deprovision your container and stop the token transfer. This is a critical step to conserve both your tokens and testnet server capacity.
```
$ akash deployment close <deployment id> -k <key name>
```

For example:
```
$ akash deployment close 66809b2c537fcdd79bc6b5b6d28bbf2d51fbe59133a4ba0119b9e0160ab16357 -k my-key-name
Closing deployment
```


## Testnet constraints

### Supported regions
These regions are currently supported by the testnet. More will come online, so check back frequently.
 - AMS (Amsterdam, Netherlands)
 - NRT (Tokyo, Japan)
 - SJC (San Jose, California, USA)

### Uptime and availability
The Akash testnet is at an alpha-level stage of development and so **not intended for production use.**  New functionality and capacity is being added constantly, but is always presented to you as-is, so use at your own risk. Use is at our discretion and we reserve the right to bring down deployments or to re-initialize the chain at any time for any reason. 

### Resource consumption
TODO
you are limited in such and such a way - a deployment that exceeds those limits will...










