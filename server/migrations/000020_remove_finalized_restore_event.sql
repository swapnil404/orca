ALTER TABLE restore_operation_events
    DROP CONSTRAINT restore_operation_events_event_type_check;

ALTER TABLE restore_operation_events
    ADD CONSTRAINT restore_operation_events_event_type_check
    CHECK (event_type IN ('created', 'intent_changed', 'agent_report'));
