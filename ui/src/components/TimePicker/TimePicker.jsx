import { DateTimePickerComponent } from '@syncfusion/ej2-react-calendars';
import React from 'react';
import './TimePicker.scss';
import { t } from 'i18next';

class TimePicker extends React.PureComponent {
  maxDate = new Date( new Date())
  render() {
    return <DateTimePickerComponent placeholder ={t('timepicker.placeholder')} 
    id="datetimepicker" 
    strictMode={true} 
    max={this.maxDate} 
    onChange={(date) =>console.log(date)}
    />;
  }
}
export default TimePicker;
