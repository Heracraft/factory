-- Reverts 0008: build log lines go back to having no time.
alter table build_logs drop column ts;
