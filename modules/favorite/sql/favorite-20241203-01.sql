-- +migrate Up
ALTER TABLE `favorite` MODIFY COLUMN unique_key VARCHAR(200);