import { Type } from 'class-transformer';
import { IsIn, IsNumber, IsOptional, IsString, MaxLength, ValidateNested } from 'class-validator';
import { COMMAND_TYPES, CommandType } from './command.types';

export class ParamsDto {
  @IsOptional()
  @IsNumber({ allowNaN: false, allowInfinity: false })
  flow_lpm?: number;
}

export class IssueCommandDto {
  @IsIn(COMMAND_TYPES)
  type!: CommandType;

  @IsOptional()
  @ValidateNested()
  @Type(() => ParamsDto)
  params?: ParamsDto;
}

export class AckDto {
  @IsIn(['completed', 'rejected'])
  status!: 'completed' | 'rejected';

  @IsOptional()
  @IsString()
  @MaxLength(500)
  reason?: string;
}
