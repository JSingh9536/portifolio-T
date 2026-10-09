//! Ground response to subsurface injection.

/// Poisson's ratio for saturated bay mud / clayey soil.
pub const POISSON: f64 = 0.35;

/// Fraction of injected slurry volume that ends up as net ground volume
/// change. The rest is lost to dewatering and pore-space filling.
pub const VOLUME_EFFICIENCY: f64 = 0.55;

/// Vertical stress gradient of wet soil, kPa per meter of depth.
pub const OVERBURDEN_KPA_PER_M: f64 = 19.0;

/// Mogi point-source vertical surface displacement, in meters.
///
/// `dv_m3` is the source volume change at `depth_m`; `r_m` is horizontal
/// distance from the source. u_z = (1 - ν) ΔV / π · d / (r² + d²)^{3/2}
pub fn mogi_uplift_m(dv_m3: f64, depth_m: f64, r_m: f64) -> f64 {
    if depth_m <= 0.0 || dv_m3 == 0.0 {
        return 0.0;
    }
    let big_r2 = r_m * r_m + depth_m * depth_m;
    (1.0 - POISSON) * dv_m3 / std::f64::consts::PI * depth_m / (big_r2 * big_r2.sqrt())
}

/// Bottom-hole pressure needed to inject at `flow_lpm` at `depth_m`.
///
/// Overburden must be exceeded to open the injection zone; on top of that a
/// linear pipe/formation resistance term, and a stiffening term as the
/// formation fills (`filled_frac` in [0, 1]).
pub fn injection_pressure_kpa(depth_m: f64, flow_lpm: f64, filled_frac: f64) -> f64 {
    if flow_lpm <= 0.0 {
        return 0.0;
    }
    let breakdown = OVERBURDEN_KPA_PER_M * depth_m * 1.15;
    let resistance = 2.4 * flow_lpm;
    let stiffening = 1.0 + filled_frac.clamp(0.0, 1.0).powi(2);
    breakdown + resistance * stiffening
}

/// One completed or in-progress injection point.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Injection {
    pub x: f64,
    pub y: f64,
    pub depth_m: f64,
    pub volume_m3: f64,
}

/// Total surface uplift in millimetres at (x, y) from all injections.
pub fn uplift_mm_at(injections: &[Injection], x: f64, y: f64) -> f64 {
    injections
        .iter()
        .map(|inj| {
            let r = ((x - inj.x).powi(2) + (y - inj.y).powi(2)).sqrt();
            mogi_uplift_m(inj.volume_m3 * VOLUME_EFFICIENCY, inj.depth_m, r)
        })
        .sum::<f64>()
        * 1000.0
        + 0.0 // an empty f64 sum is -0.0; normalise so JSON never shows "-0"
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mogi_peak_matches_closed_form() {
        // r = 0 => (1-ν) ΔV / (π d²)
        let got = mogi_uplift_m(10.0, 5.0, 0.0);
        let want = (1.0 - POISSON) * 10.0 / (std::f64::consts::PI * 25.0);
        assert!((got - want).abs() < 1e-12);
    }

    #[test]
    fn mogi_decays_monotonically_with_distance() {
        let mut last = f64::INFINITY;
        for r in 0..50 {
            let u = mogi_uplift_m(5.0, 10.0, r as f64);
            assert!(u < last);
            last = u;
        }
    }

    #[test]
    fn mogi_surface_volume_is_two_one_minus_nu_dv() {
        // ∫∫ u_z dA = 2 (1 - ν) ΔV for a Mogi source. Integrate radially.
        let (dv, d) = (3.0, 8.0);
        let dr = 0.01;
        let mut vol = 0.0;
        let mut r = dr / 2.0;
        while r < 20_000.0 {
            vol += mogi_uplift_m(dv, d, r) * 2.0 * std::f64::consts::PI * r * dr;
            r += dr;
        }
        let want = 2.0 * (1.0 - POISSON) * dv;
        assert!((vol - want).abs() / want < 1e-3, "vol={vol} want={want}");
    }

    #[test]
    fn deeper_sources_spread_wider_but_lower() {
        let shallow = mogi_uplift_m(1.0, 5.0, 0.0);
        let deep = mogi_uplift_m(1.0, 20.0, 0.0);
        assert!(shallow > deep);
        assert!(mogi_uplift_m(1.0, 20.0, 25.0) > mogi_uplift_m(1.0, 5.0, 25.0));
    }

    #[test]
    fn pressure_rises_with_depth_flow_and_fill() {
        assert_eq!(injection_pressure_kpa(10.0, 0.0, 0.0), 0.0);
        let base = injection_pressure_kpa(10.0, 100.0, 0.0);
        assert!(injection_pressure_kpa(20.0, 100.0, 0.0) > base);
        assert!(injection_pressure_kpa(10.0, 150.0, 0.0) > base);
        assert!(injection_pressure_kpa(10.0, 100.0, 0.9) > base);
    }

    #[test]
    fn no_injections_is_positive_zero() {
        assert!(uplift_mm_at(&[], 0.0, 0.0).is_sign_positive());
    }

    #[test]
    fn superposition_adds() {
        let a = Injection {
            x: 0.0,
            y: 0.0,
            depth_m: 10.0,
            volume_m3: 4.0,
        };
        let b = Injection {
            x: 6.0,
            y: 0.0,
            depth_m: 10.0,
            volume_m3: 4.0,
        };
        let sum = uplift_mm_at(&[a, b], 3.0, 0.0);
        let parts = uplift_mm_at(&[a], 3.0, 0.0) + uplift_mm_at(&[b], 3.0, 0.0);
        assert!((sum - parts).abs() < 1e-12);
    }
}
