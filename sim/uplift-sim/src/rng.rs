//! Tiny deterministic PRNG (xorshift64*) so runs are reproducible by seed
//! without pulling in `rand`.

#[derive(Debug, Clone)]
pub struct Rng(u64);

impl Rng {
    pub fn new(seed: u64) -> Self {
        Self(seed.max(1))
    }

    pub fn next_u64(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545_F491_4F6C_DD1D)
    }

    /// Uniform in [0, 1).
    pub fn unit(&mut self) -> f64 {
        (self.next_u64() >> 11) as f64 / (1u64 << 53) as f64
    }

    /// Approximately normal noise with the given standard deviation.
    pub fn noise(&mut self, sd: f64) -> f64 {
        // Irwin–Hall with n=12: sum of 12 uniforms minus 6 has unit variance.
        let z: f64 = (0..12).map(|_| self.unit()).sum::<f64>() - 6.0;
        z * sd
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn deterministic_and_bounded() {
        let (mut a, mut b) = (Rng::new(42), Rng::new(42));
        for _ in 0..1000 {
            let u = a.unit();
            assert_eq!(u, b.unit());
            assert!((0.0..1.0).contains(&u));
        }
    }

    #[test]
    fn noise_has_roughly_requested_sd() {
        let mut r = Rng::new(7);
        let n = 20_000;
        let xs: Vec<f64> = (0..n).map(|_| r.noise(2.0)).collect();
        let mean = xs.iter().sum::<f64>() / n as f64;
        let var = xs.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / n as f64;
        assert!(mean.abs() < 0.1, "mean={mean}");
        assert!((var.sqrt() - 2.0).abs() < 0.1, "sd={}", var.sqrt());
    }
}
