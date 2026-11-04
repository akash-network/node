import base64
import hashlib
import json
from pathlib import Path
from decimal import Decimal

DIRECTORY = Path(__file__).resolve().parent
ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"


def base58(value):
    number = int.from_bytes(value, "big")
    encoded = ""
    while number:
        number, digit = divmod(number, 58)
        encoded = ALPHABET[digit] + encoded
    return "1" * (len(value) - len(value.lstrip(b"\x00"))) + encoded


def varint(value):
    output = bytearray()
    while value >= 128:
        output.append(value & 127 | 128)
        value >>= 7
    output.append(value)
    return bytes(output)


def scalar(field, value):
    value = int(value)
    return varint(field * 8) + varint(value) if value else b""


def message(field, value):
    return varint(field * 8 + 2) + varint(len(value)) + value


def string(field, value):
    return message(field, value.encode()) if value else b""


def policy(value):
    return (scalar(1, value["version"])
            + b"".join(string(2, member) for member in value["members"])
            + string(3, str(int(Decimal(value["threshold"]) * 10**18))))


def observation(value):
    outcome = {"OUTCOME_UNSPECIFIED": 0, "OUTCOME_MINTED": 1, "OUTCOME_VOIDED": 2}[value["outcome"]]
    signature = base64.b64decode(value["transaction_signature"], validate=True)
    return (scalar(1, outcome) + scalar(2, value["slot"])
            + (message(3, signature) if signature else b""))


def vote(value):
    return (string(1, value["reporter"]) + message(2, observation(value["observation"]))
            + scalar(3, value["policy_version"]))


def settlement(value):
    hold = {"HOLD_NONE": 0, "HOLD_VETO": 1, "HOLD_CONFLICT": 2}[value["hold"]]
    encoded = (scalar(1, hold) + scalar(2, value["round"]) + message(3, policy(value["policy"]))
               + b"".join(message(4, vote(item)) for item in value["votes"])
               + scalar(5, value["challenge_end"]))
    if value["governance_observation"] is not None:
        encoded += message(6, observation(value["governance_observation"]))
    encoded += string(7, value["veto_by"]) + string(8, value["veto_reason"])
    if value["void_authorization"] is not None:
        receipt = value["void_authorization"]
        encoded += message(9, scalar(1, receipt["requested_height"]) + scalar(2, receipt["requested_round"]))
    return encoded + scalar(10, value["veto_policy_version"])


def identity(value):
    return (bytes([len(bytes.fromhex(value["owner_hex"]))]) + bytes.fromhex(value["owner_hex"])
            + int(value["height"]).to_bytes(8, "big") + int(value["sequence"]).to_bytes(8, "big"))


boundaries = json.loads((DIRECTORY / "native_boundary_vectors.json").read_text())

for fixture in json.loads((DIRECTORY / "proof_vectors.json").read_text()) + boundaries["records"]:
    recipient = base58(bytes.fromhex(fixture["recipient_hex"]))
    assert recipient == fixture["recipient"]
    amount = str(int(fixture["amount"]))
    assert 0 < int(amount) <= 2**64 - 1
    value = string(1, recipient) + message(2, string(1, "uakt") + string(2, amount))
    key = bytes([2, 0]) + identity(fixture)
    assert key.hex() == fixture["transfer_key_hex"]
    assert value.hex() == fixture["record_value_hex"]
    assert hashlib.sha256(key).hexdigest() == fixture["mint_proof_nonce_hex"]
    print(f"native_record name={fixture['name']} bytes={len(value)} key_nonce=matched independent_base58=true")

for fixture in json.loads((DIRECTORY / "settlement_vectors.json").read_text()) + boundaries["settlements"]:
    key = bytes([3, 1, fixture["status"]]) + identity(fixture)
    value = settlement(fixture["settlement"])
    assert key.hex() == fixture["settlement_key_hex"]
    assert value.hex() == fixture["settlement_value_hex"]
    assert hashlib.sha256(key).hexdigest() == fixture["proof_nonce_hex"]
    print(f"native_settlement name={fixture['name']} bytes={len(value)} key_nonce=matched full_value=matched")
