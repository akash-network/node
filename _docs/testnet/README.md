#The Akash Testnet

### About the testnet
 - A basic MVP with support for requesting, depploying, and paying for deployments 
 - We want your feedback!  Please message us [on Telegram](https://t.me/AkashNW) with any and all of your feedback.  We're putting our in-progess platform out there so that we can get real-world feedback, so don't be shy!
 - The Akash testnet is a fully-functioning decentralized cloud, but is at an alpha-level stage of development and so **not intended for production use.**  It's presented to you as-is, use at your own risk
 - Capacity provided by Packet, with more coming online as needed
 - Use is at our discretion and we reserve the right to bring down deployments at any time for any reason
 - 

### Intended users
Fundamentally, the Akash testnet is a deployment platform with a CLI and intended for relatively sophisticated users.  If you are comfortable managing instances via the AWS API and can build and deploy a Docker container, you will find the Akash testnet easy to use.  If not, please feel free to give it a shot, but you might find it confusing.

### Getting help
First of course, run `akash -h` and RTFM, then please feel free to ask questions in our Telegram channel https://t.me/AkashNW.


### Regions
 - sjc
 - nrt


### Behavioral guidelines
 - As mentioned, use is at our discretion
 - Testnet capacity is limited so, although it's free, please treat it as the scarce community resource it is

## Getting started
The testnet supports this basic workflow:
 1. Register and receive testnet tokens
 1. Deploy your image
 1. Close your deployment (i.e. deprovision containers)

The sections below describe each step.


#### Testnet registration
 - You must register to request access.
 - After registering, you will receive a set of Akash testnet tokens **Please note that testnet tokens are only usable on the Akash testnet and have no market value.**
 - The registration process looks like this:
   1. Go to https://akash.network/testnet/ and follow the instructions.
   1. You'll immediately receive a confirmation email.  Click the link inside to confirm your email and request testnet tokens.
   1. After review by our staff, we will send tokens to your address and confirm with another email.
 - If you wish to receive another token allocation, repeat these steps and we will happily consider your request


### Deploy your image
 1. Check your balance
```
$ akash key list #returns your key names and values
$ akash query account <key value> #returns your balance
```
 1. Download the [sample deployment file](xxx) and put it somewhere convenient.
 1. Modify the sample deployment file if desired. 
   - The sample deployment file specifies a small webapp container running a simple demo site we created.  Feel free to modify the image location and other values according to our [SDL (Stack Definition Language) documentation](../sdl.md).
   - You are limited in the amount of testnet resources you may request. Please see the [Limitations section](#Testnet-Limitations).
 1. Create a deployment request
 ```
 $ akash key list
 $ akash deploy <deployment file path> -k <key name> #creates and sends deployment
 ```
 1. The CLI client will print the deployment address, bid and lease data to console. You may also query your deployments
 ```
 $ akash query deployment #returns all your deployments
 $ akash query deployment <deployment address> #returns the deployment located at <deployment address>
 ```
 1. Your container will automatically deploy and run. You may also check its status
 ```
 akash something
 ```
 1. Access your deployed application in whatever way makes sense to you
 1. Check your balance, you will see tokens being transferred to the provider
 ```
 $ akash query account
```

### Close your deployment

 1. When you are done with your application, close the deployment. This will deprovision your container and stop the token transfer
```
$ akash deployment close <deployment address> -k <key name>
```



### Advanced commands
checking providers


### Testnet Limitations


single container, multi connect over 80

you are limited in such and such a way - a deployment that exceeds those limits will...


you have container and registry
