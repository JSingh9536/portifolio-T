//! Physics-backed simulator for a fleet of subsurface slurry-injection robots.
//!
//! Each robot drills to a target depth, injects wood-waste slurry, and the
//! resulting volume change lifts the ground surface. Uplift is modeled with
//! the Mogi point source in an elastic half-space, superposed over every
//! injection on the site.

pub mod command;
pub mod physics;
pub mod rng;
pub mod robot;
pub mod site;
