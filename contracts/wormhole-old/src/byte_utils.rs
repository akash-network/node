use cosmwasm_std::CanonicalAddr;

pub trait ByteUtils {
    fn get_u8(&self, index: usize) -> u8;
    fn get_u16(&self, index: usize) -> u16;
    fn get_u32(&self, index: usize) -> u32;
    fn get_u64(&self, index: usize) -> u64;
    fn get_u128(&self, index: usize) -> u128;
    fn get_u256(&self, index: usize) -> (u128, u128);
    fn get_bytes32(&self, index: usize) -> &[u8];
    fn get_address(&self, index: usize) -> CanonicalAddr;
}

impl ByteUtils for &[u8] {
    fn get_u8(&self, index: usize) -> u8 {
        self[index]
    }

    fn get_u16(&self, index: usize) -> u16 {
        let mut bytes = [0u8; 2];
        bytes.copy_from_slice(&self[index..index + 2]);
        u16::from_be_bytes(bytes)
    }

    fn get_u32(&self, index: usize) -> u32 {
        let mut bytes = [0u8; 4];
        bytes.copy_from_slice(&self[index..index + 4]);
        u32::from_be_bytes(bytes)
    }

    fn get_u64(&self, index: usize) -> u64 {
        let mut bytes = [0u8; 8];
        bytes.copy_from_slice(&self[index..index + 8]);
        u64::from_be_bytes(bytes)
    }

    fn get_u128(&self, index: usize) -> u128 {
        let mut bytes = [0u8; 16];
        bytes.copy_from_slice(&self[index..index + 16]);
        u128::from_be_bytes(bytes)
    }

    fn get_u256(&self, index: usize) -> (u128, u128) {
        (self.get_u128(index), self.get_u128(index + 16))
    }

    fn get_bytes32(&self, index: usize) -> &[u8] {
        &self[index..index + 32]
    }

    fn get_address(&self, index: usize) -> CanonicalAddr {
        CanonicalAddr::from(&self[index + 12..index + 32])
    }
}

pub fn extend_address_to_32(addr: &CanonicalAddr) -> Vec<u8> {
    let mut result = vec![0u8; 32];
    let addr_bytes = addr.as_slice();
    let start = 32 - addr_bytes.len();
    result[start..].copy_from_slice(addr_bytes);
    result
}
