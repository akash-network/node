pragma solidity ^0.4.18;

contract BadActor {
    bool public isBadActor
}

contract Matchable {
    bool public matched;
    function Matchable() public {
        matched = false;
    }
    modifier notMatched() public {
        require(!matched)
    }
}

contract Cancelable {
    bool public canceled;
    modifier isCanceled() {
        require(canceled);
        _;
    }
    modifier notCanceled() {
        require(!canceled);
        _;
    }
}

contract Payable {
    function () public payable {}
}

contract Ownable {
    address public owner;
    function Ownable() public {
        owner = msg.sender;
    }
    modifier onlyOwner() {
        require(msg.sender == owner);
        _;
    }
}

contract Deliquent {
    bool public deliquent
    function Deliquent() public {
        deliquent = false;
    }
    modifier isDelinquent() {
        require(deliquent)
        _;
    }
    modifier notDelinquent() {
        require(!deliquent)
        _;
    }
}

contract Maintainable {
    address public maintainer;
    function Maintainable(address rootCaller) public {
      maintainer = rootCaller;
    }
    modifier onlyMaintainer() {
        require(msg.sender == maintainer);
        _;
    }
    function transferMaintainorship(address newMaintainer) public onlyMaintainer {
        if (newMaintainer != address(0)) {
          maintainer = newMaintainer;
        }
    }
 }

contract Parameterized is Maintainable {

    uint public ram;
    uint public cpu;
    uint public rate;
    uint public minimumBalanace;
    uint public cancelFee;

    function Parameterized(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee) public {
        ram = _ram;
        cpu = _cpu;
        rate = _rate;
        minimumBalanace = _minimumBalanace;
        cancelFee = _cancelFee;
    }
    function alter(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee) public onlyMaintainer {
        ram = _ram;
        cpu = _cpu;
        rate = _rate;
        minimumBalanace = _minimumBalanace;
        cancelFee = _cancelFee;
    }
}

contract Provider is Ownable, Parameterized, Matchable, Cancelable, Payable, BadActor {

    // address of matched client
    Client public matchedClient;
    // Provider's public endpoint for Manifest distribution
    string public networkAddress;

    // Constructor. Ensure contract has value >= cancelFee
    function Provider(address maintainer, uint ram, uint cpu, uint rate, uint minimumBalanace, uint cancelFee) public Parameterized(ram, cpu, rate, minimumBalanace, cancelFee) Maintainable(maintainer) {}

    // confirm and declare that a valid matching contract is at an address
    function match(address clientAddress) public onlyOwner notCanceled notMatched returns (bool) {
        // load client contract
        Client client = Client(clientAddress);
        // make sure resources match
        require(client.cpu() <= cpu && client.ram() <= ram && client.rate() >= rate && client.minimumBalanace() >= minimumBalanace && client.cancelFee() <= cancelFee);
        // attempt to match with the client
        require(client.match(this))
        matchedClient = client;
        matched = true;
        return matched;
    }

    // reset contract fields. Charge fee if early cancelation. Sends contract balance to maintainer
    function cancel() public notCanceled onlyMaintainer {
        matched = false;
        canceled = true;

        // if the matchedClients balance has falled below the minium balance do not charge an early cancellation fee
        if (matchedClient.balance >= matchedClient.minimumBalanace()) {
            matchedClient.transfer(cancelFee);
        }
        maintainer.transfer(this.balance);
    }

    // makes canceled false. contract can be rematched to a provider.
    function uncancel() public isCanceled   {
        canceled = false;
    }

    // send funds to contract maintainer
    function withdrawal() public {
        uint amount = this.balance - cancelFee;
        // ensure contract always can pay the early cancel fee
        require(amount > 0);
        maintainer.transfer(amount);
    }
  }

 contract Client is Ownable, Parameterized, Matchable, Cancelable, Payable, Deliquent {

    Provider public matchedProvider;
    uint public minimumBalanace;
    uint public matchStartTime;
    uint public totalBilled;
    uint public unsettledBalance;
    uint public maxUnsettledBalance;

    // Constructor
    function Client(address maintainer, uint ram, uint cpu, uint rate, uint _minimumBalanace, uint cancelFee, uint maxUnsettledBalance) public Parameterized(ram, cpu, rate, _minimumBalanace, cancelFee) Maintainable(maintainer) {}

    // confirm and delcare that the matching contracts matching contract address is this address.
    // confirms both contracts agree that they can match
    function match(address providerAddress) public onlyOwner notCanceled notMatched returns (bool) {
        // load provider contract
        Provider provider = Provider(providerAddress);
        // check provider is compatible
        require(provider.cpu() >= cpu && provider.ram() >= ram && provider.rate() <= rate && provider.minimumBalanace() <= minimumBalanace && provider.cancelFee() >= cancelFee);
        matchedProvider = provider;
        matchStartTime = now;
        matched = true;
        return matched;
    }

    // create a bill for the client.
    function setBill() public onlyOwner {
        // pay required amount since last payment time
        // get the required payment by total payment - expected payment
        uint expectedBilled = (now - matchStartTime) * rate;
        unsettledBalance = expectedBilled - totalBilled;
    }

    // send the value of the outstanding bill to the provider
    function bill() public notCanceled {
        // pay the provider
        uint payment;
        if (this.balance > unsettledBalance) {
            payment = unsettledBalance;
        } else {
           payment = this.balance;
        }
        matchedProvider.transfer(payment);
        // update the total amount billed and unsetted
        totalBilled += payment;
        unsettledBalance -= payment;

        // if unselted balance is > N hour value of contract let provider cancel it
        if (ununsettledBalance > maxUnsettledBalance) {
            delinquent = true;
        }
    }

    // reset contract fields. Prevents payments. Sends contract balance to maintainer
    function cancel() public onlyMaintainer notCanceled notDelinquent {
        matched = false;
        canceled = true;
        maintainer.transfer(this.balance);
    }

    // lets the provider cancel the contract for deliquent payment
    // leave delinquent true so there is a perminent record of clients bad behavior
    function providerCancel() public isDelinquent {
        // only matchedProvider should be able to call this
        require(msg.sender == matchedProvider)
        matched = false;
        canceled = true;
        matchedProvider.transfer(this.balance);
    }

    // makes canceled false. contract can be rematched to a provider.
    function uncancel() public isCanceled notDelinquent{
        canceled = false;
    }
  }

  contract Master is Maintainable {

    function Master() public Maintainable(msg.sender) {}

    // call to put a bid for a service on the network
    function deployProvider(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee) public payable returns (Provider) {
        // provider must provide the cancel fee up front
        require(msg.value >= _cancelFee);
        Provider provider = new Provider(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee);
        provider.transfer(msg.value);
        return provider;
    }

    // call to put an ask for a service on the network
    function deployClient(uint _ram, uint _cpu, uint _rate, uint _minimumBalanace, uint _cancelFee) public payable returns (Client) {
        // ensure client contract has minimum balance
        require(msg.value >= _minimumBalanace);
        Client client = new Client(msg.sender, _ram, _cpu, _rate, _minimumBalanace, _cancelFee);
        client.transfer(msg.value);
        return client;
    }

    // try to maych a client and a provider
    // this is onlyMaintainer because the matching must be done in a fair way
    // photon will do the matching based on provider and client order sequennces
    // the results of the matching are publicly auditable because eth transactions are public
    function match(address clientAddress, address providerAddress) onlyMaintainer public {
      // Matches initated by providers. Could be the other way around, makes no difference
      Provider provider = Provider(providerAddress);
      require(provider.match(clientAddress));
    }
  }
