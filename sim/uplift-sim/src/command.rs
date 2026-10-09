//! Operator commands delivered by the mission-control service.

use serde::{Deserialize, Serialize};

/// Wire format matches mission-control's `RobotCommand`:
/// `{"id": "...", "type": "set_flow", "params": {"flow_lpm": 90}}`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Envelope {
    pub id: String,
    #[serde(flatten)]
    pub command: Command,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", content = "params", rename_all = "snake_case")]
pub enum Command {
    Pause,
    Resume,
    Estop,
    ClearFault,
    SetFlow { flow_lpm: f64 },
}

/// Robot's reply once it has applied (or refused) a command.
#[derive(Debug, Clone, PartialEq, Serialize)]
#[serde(rename_all = "snake_case", tag = "status")]
pub enum Ack {
    Completed,
    Rejected { reason: String },
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_unit_and_param_commands() {
        let e: Envelope = serde_json::from_str(r#"{"id":"c1","type":"estop"}"#).unwrap();
        assert_eq!(e.command, Command::Estop);
        let e: Envelope =
            serde_json::from_str(r#"{"id":"c2","type":"set_flow","params":{"flow_lpm":90}}"#)
                .unwrap();
        assert_eq!(e.command, Command::SetFlow { flow_lpm: 90.0 });
        // Extra fields from mission-control (robot_id, status, timestamps) are ignored.
        let e: Envelope = serde_json::from_str(
            r#"{"id":"c3","robot_id":"tn-01","type":"pause","params":null,"status":"sent"}"#,
        )
        .unwrap();
        assert_eq!(e.command, Command::Pause);
    }

    #[test]
    fn ack_serializes_with_status_tag() {
        let j = serde_json::to_string(&Ack::Rejected {
            reason: "nope".into(),
        })
        .unwrap();
        assert_eq!(j, r#"{"status":"rejected","reason":"nope"}"#);
        assert_eq!(
            serde_json::to_string(&Ack::Completed).unwrap(),
            r#"{"status":"completed"}"#
        );
    }
}
