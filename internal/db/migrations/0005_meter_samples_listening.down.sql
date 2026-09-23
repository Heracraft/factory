-- Reverts 0005_meter_samples_listening.
alter table meter_samples drop column listening;
